// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

type ethernetMenuAction int

const (
	ethernetMenuConnect ethernetMenuAction = iota
	ethernetMenuDisconnect
	ethernetMenuEdit
	ethernetMenuBack
)

type ethernetMenuOption struct {
	label  string
	action ethernetMenuAction
}

type ethernetActionMenu struct {
	device  model.Device
	profile *model.ConnectionProfile
	options []ethernetMenuOption
	cursor  int
	busy    bool
	notice  string
	err     error
}

type ethernetOperationMsg struct {
	notice   string
	err      error
	snapshot *model.Snapshot
}

func newEthernetActionMenu(device model.Device, profile *model.ConnectionProfile) *ethernetActionMenu {
	menu := &ethernetActionMenu{device: device, profile: profile}
	if device.ActiveConnection != nil {
		menu.options = append(menu.options, ethernetMenuOption{label: "Disconnect", action: ethernetMenuDisconnect})
	} else if profile != nil {
		menu.options = append(menu.options, ethernetMenuOption{label: "Connect", action: ethernetMenuConnect})
	}
	label := "Configure Ethernet"
	if profile != nil {
		label = "Edit settings"
	}
	menu.options = append(menu.options,
		ethernetMenuOption{label: label, action: ethernetMenuEdit},
		ethernetMenuOption{label: "Back", action: ethernetMenuBack},
	)
	return menu
}

func (m *Model) updateEthernetMenu(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case ethernetOperationMsg:
		m.ethernetMenu.busy = false
		if msg.snapshot != nil {
			m.snapshot = *msg.snapshot
			m.rebuildEthernetMenu()
		}
		if msg.err != nil {
			m.ethernetMenu.err = msg.err
			return false, nil
		}
		m.ethernetMenu.notice = msg.notice
		m.ethernetMenu.err = nil
		return false, nil

	case tea.KeyPressMsg:
		if m.ethernetMenu.busy {
			return false, nil
		}
		switch msg.String() {
		case "esc":
			return true, nil
		case "up", "k":
			if m.ethernetMenu.cursor > 0 {
				m.ethernetMenu.cursor--
			}
		case "down", "j":
			if m.ethernetMenu.cursor+1 < len(m.ethernetMenu.options) {
				m.ethernetMenu.cursor++
			}
		case "enter":
			option := m.ethernetMenu.options[m.ethernetMenu.cursor]
			switch option.action {
			case ethernetMenuBack:
				return true, nil
			case ethernetMenuEdit:
				device := m.ethernetMenu.device
				m.ethernetMenu = nil
				return false, m.openEthernetForm(device)
			case ethernetMenuConnect:
				if m.ethernetMenu.profile == nil {
					m.ethernetMenu.err = fmt.Errorf("no saved Ethernet profile is available")
					return false, nil
				}
				m.ethernetMenu.busy = true
				return false, m.activateEthernet(*m.ethernetMenu.profile, m.ethernetMenu.device)
			case ethernetMenuDisconnect:
				m.ethernetMenu.busy = true
				return false, m.disconnectEthernet(m.ethernetMenu.device)
			}
		}
	}
	return false, nil
}

func (m Model) activateEthernet(profile model.ConnectionProfile, device model.Device) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		callErr := m.source.ActivateEthernetProfile(ctx, profile.ObjectPath, device.ObjectPath)
		var snapshot model.Snapshot
		var stateErr error
		if callErr != nil {
			var current model.Device
			snapshot, current, stateErr = currentDeviceSnapshot(ctx, m.source, device.ObjectPath, device.Interface)
			if stateErr == nil && activationMatches(current, profile.UUID, "") {
				callErr = nil
			}
		} else {
			snapshot, _, stateErr = waitForDeviceActivation(
				ctx,
				m.source,
				device.ObjectPath,
				device.Interface,
				profile.UUID,
				"",
			)
		}
		return ethernetOperationMsg{
			notice:   "Connected using saved Ethernet profile " + profile.ID + ".",
			err:      operationStateError("connect Ethernet", callErr, stateErr),
			snapshot: &snapshot,
		}
	}
}

func (m Model) disconnectEthernet(device model.Device) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		callErr := m.source.DisconnectEthernet(ctx, device.ObjectPath)
		snapshot, _, stateErr := waitForDeviceDisconnected(
			ctx,
			m.source,
			device.ObjectPath,
			device.Interface,
		)
		return ethernetOperationMsg{
			notice:   "Ethernet disconnected.",
			err:      operationStateError("disconnect Ethernet", callErr, stateErr),
			snapshot: &snapshot,
		}
	}
}

func (m *Model) rebuildEthernetMenu() {
	if m.ethernetMenu == nil {
		return
	}
	device, ok := findDevice(
		m.snapshot.Devices,
		m.ethernetMenu.device.ObjectPath,
		m.ethernetMenu.device.Interface,
	)
	if !ok {
		return
	}
	notice := m.ethernetMenu.notice
	menu := newEthernetActionMenu(device, m.preferredEthernetProfile(device))
	menu.notice = notice
	m.ethernetMenu = menu
}

func (m Model) renderEthernetMenu(width int) string {
	menu := m.ethernetMenu
	var out strings.Builder
	out.WriteString(titleStyle.Render("Ethernet · " + emptyFallback(menu.device.Interface, "adapter")))
	out.WriteString("\n\n")
	out.WriteString(titleStyle.Render("Actions"))
	out.WriteString("\n\n")

	var lines []string
	for index, option := range menu.options {
		marker := "  "
		if index == menu.cursor {
			marker = "› "
		}
		lines = append(lines, marker+option.label)
	}
	out.WriteString(selectedCardStyle.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n")))
	if menu.err != nil {
		out.WriteString("\n\n")
		out.WriteString(errorStyle.Render(menu.err.Error()))
	}
	if menu.notice != "" {
		out.WriteString("\n\n")
		out.WriteString(goodStyle.Render(menu.notice))
	}
	if menu.busy {
		out.WriteString("\n\n")
		out.WriteString(warningStyle.Render("Working..."))
	}
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Render("↑/↓ choose   Enter select   Esc cancel"))
	return out.String()
}
