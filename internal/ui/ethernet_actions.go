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
	err     error
}

type ethernetOperationMsg struct {
	notice string
	err    error
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
		if msg.err != nil {
			m.ethernetMenu.err = msg.err
			return false, nil
		}
		m.notice = msg.notice
		m.loading = true
		return true, m.loadSnapshot()

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
		if err := m.source.ActivateEthernetProfile(ctx, profile.ObjectPath, device.ObjectPath); err != nil {
			return ethernetOperationMsg{err: err}
		}
		return ethernetOperationMsg{notice: "Connected using saved Ethernet profile " + profile.ID + "."}
	}
}

func (m Model) disconnectEthernet(device model.Device) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := m.source.DisconnectEthernet(ctx, device.ObjectPath); err != nil {
			return ethernetOperationMsg{err: err}
		}
		return ethernetOperationMsg{notice: "Ethernet disconnected."}
	}
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
	if menu.busy {
		out.WriteString("\n\n")
		out.WriteString(warningStyle.Render("Working..."))
	}
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Render("↑/↓ choose   Enter select   Esc cancel"))
	return out.String()
}
