// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

type privateAccessSnapshotMsg struct {
	snapshot model.Snapshot
}

type privateAccessSnapshotErrMsg struct {
	err error
}

type privateAccessScreen struct {
	source   networkSnapshotSource
	snapshot model.Snapshot
	loading  bool
	err      error
}

func newPrivateAccessScreen(source networkSnapshotSource, snapshot model.Snapshot) *privateAccessScreen {
	return &privateAccessScreen{
		source:   source,
		snapshot: snapshot,
	}
}

func (s *privateAccessScreen) update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case privateAccessSnapshotMsg:
		s.snapshot = msg.snapshot
		s.loading = false
		s.err = nil

	case privateAccessSnapshotErrMsg:
		s.loading = false
		s.err = msg.err

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc":
			return true, nil
		case "r":
			if !s.loading {
				s.loading = true
				s.err = nil
				return false, s.loadSnapshot()
			}
		}
	}

	return false, nil
}

func (s *privateAccessScreen) loadSnapshot() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		snapshot, err := s.source.Snapshot(ctx)
		if err != nil {
			return privateAccessSnapshotErrMsg{err: err}
		}
		return privateAccessSnapshotMsg{snapshot: snapshot}
	}
}

func (s *privateAccessScreen) render(width, height int) string {
	if width < 30 {
		width = 30
	}
	contentWidth := width

	var out strings.Builder
	header := titleStyle.Render("Private Access") + "  " + mutedStyle.Render("Tailscale and NetBird")
	out.WriteString(lipgloss.NewStyle().Width(contentWidth).Padding(0, 1).Render(header))
	out.WriteString("\n")
	out.WriteString(lipgloss.NewStyle().Width(contentWidth).Padding(0, 1).Render(
		mutedStyle.Render("Read-only provider status. Connection and login controls are not enabled yet."),
	))

	if s.loading {
		out.WriteString("\n\n  " + warningStyle.Render("Refreshing private access state..."))
	} else if s.err != nil {
		out.WriteString("\n\n  " + errorStyle.Render("Private access refresh failed"))
		out.WriteString("\n  " + mutedStyle.Render(s.err.Error()))
	} else {
		out.WriteString("\n")
		providers := s.snapshot.VPN
		if len(providers) == 0 {
			out.WriteString("\n")
			out.WriteString(cardStyle.
				Width(cardContentWidth(contentWidth)).
				Padding(1, 2).
				Render("Private access provider state is unavailable."))
		} else {
			for _, provider := range providers {
				out.WriteString("\n")
				out.WriteString(renderPrivateAccessProvider(contentWidth, provider))
			}
		}
	}

	out.WriteString("\n\n")
	out.WriteString(helpStyle.Width(contentWidth).Render("Esc/q back   r refresh"))
	return out.String()
}

func renderPrivateAccessProvider(width int, provider model.VPNProviderState) string {
	name := emptyFallback(provider.Name, string(provider.ID))
	lines := []string{titleStyle.Render(name)}

	if !provider.Installed {
		lines = append(lines, "  "+mutedStyle.Render("Not installed"))
		return cardStyle.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n"))
	}

	serviceEnabled := "disabled"
	if provider.ServiceEnabled {
		serviceEnabled = "enabled"
	}
	serviceState := emptyFallback(provider.ServiceState, "unknown")
	serviceLine := fmt.Sprintf("  Service     %s · %s", serviceEnabled, serviceState)
	lines = append(lines, serviceLine)

	connectionLabel := emptyFallback(provider.ConnectionState, "unknown")
	switch {
	case provider.Connected:
		connectionLabel = goodStyle.Render("connected") + " · " + connectionLabel
	case provider.ConnectionState == "unavailable":
		connectionLabel = mutedStyle.Render("details unavailable")
	case provider.ServiceRunning:
		connectionLabel = warningStyle.Render(connectionLabel)
	default:
		connectionLabel = mutedStyle.Render(connectionLabel)
	}
	lines = append(lines, "  Connection  "+connectionLabel)

	if len(provider.Addresses) > 0 {
		lines = append(lines, "  Addresses   "+strings.Join(provider.Addresses, ", "))
	}
	if provider.StatusError != "" {
		lines = append(lines, "  "+errorStyle.Render(provider.StatusError))
	}

	return cardStyle.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n"))
}

func privateAccessSummary(snapshot model.Snapshot) string {
	if len(snapshot.VPN) == 0 {
		return "Tailscale and NetBird status unavailable"
	}

	parts := make([]string, 0, len(snapshot.VPN))
	for _, provider := range snapshot.VPN {
		name := emptyFallback(provider.Name, string(provider.ID))
		state := "not installed"
		switch {
		case !provider.Installed:
		case provider.Connected:
			state = "connected"
		case provider.ServiceRunning && provider.ConnectionState == "unavailable":
			state = "running"
		case provider.ServiceRunning:
			state = emptyFallback(provider.ConnectionState, "running")
		default:
			state = "stopped"
		}
		parts = append(parts, name+" "+state)
	}
	return strings.Join(parts, " · ")
}
