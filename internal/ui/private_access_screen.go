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

type privateAccessSource interface {
	networkSnapshotSource
	VPNAction(context.Context, model.VPNProviderID, model.VPNAction) (model.VPNActionResult, error)
	VPNWaitAuthentication(context.Context, model.VPNProviderID, string) (model.VPNActionResult, error)
}

type privateAccessSnapshotMsg struct {
	snapshot model.Snapshot
}

type privateAccessSnapshotErrMsg struct {
	err error
}

type privateAccessActionMsg struct {
	provider model.VPNProviderID
	action   model.VPNAction
	result   model.VPNActionResult
}

type privateAccessActionErrMsg struct {
	err error
}

type privateAccessConnectionMsg struct {
	snapshot model.Snapshot
	settled  bool
}

type privateAccessConnectionErrMsg struct {
	err error
}

type privateAccessAuthMsg struct {
	provider model.VPNProviderID
	result   model.VPNActionResult
}

type privateAccessAuthErrMsg struct {
	err error
}

type privateAccessActionOption struct {
	label  string
	action model.VPNAction
}

type privateAccessAuthState struct {
	provider model.VPNProviderID
	url      string
	userCode string
}

type privateAccessScreen struct {
	source       privateAccessSource
	snapshot     model.Snapshot
	cursor       int
	actionCursor int
	selectedID   model.VPNProviderID
	inActions    bool
	loading      bool
	acting       bool
	authWaiting      bool
	waitingConnection bool
	auth             *privateAccessAuthState
	authCancel   context.CancelFunc
	err          error
	notice       string
}

func newPrivateAccessScreen(source privateAccessSource, snapshot model.Snapshot) *privateAccessScreen {
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
		s.clampCursors()

	case privateAccessSnapshotErrMsg:
		s.loading = false
		s.err = msg.err

	case privateAccessActionMsg:
		s.err = nil
		s.notice = msg.result.Message
		if msg.result.AwaitingAuth {
			s.acting = false
			s.waitingConnection = false
			s.auth = &privateAccessAuthState{
				provider: msg.provider,
				url:      msg.result.AuthURL,
				userCode: msg.result.UserCode,
			}
			return false, s.startAuthenticationWait()
		}
		if actionWaitsForConnection(msg.action) {
			s.acting = true
			s.waitingConnection = true
			s.loading = true
			return false, s.waitForProviderConnection(msg.provider)
		}
		s.acting = false
		s.waitingConnection = false
		s.loading = true
		return false, s.loadSnapshot()

	case privateAccessActionErrMsg:
		s.acting = false
		s.waitingConnection = false
		s.err = msg.err

	case privateAccessConnectionMsg:
		s.snapshot = msg.snapshot
		s.acting = false
		s.waitingConnection = false
		s.loading = false
		s.err = nil
		s.clampCursors()
		if !msg.settled {
			s.notice = "Connection is still starting. Press r to refresh."
		}

	case privateAccessConnectionErrMsg:
		s.acting = false
		s.waitingConnection = false
		s.loading = false
		s.err = msg.err

	case privateAccessAuthMsg:
		s.clearAuthWait()
		s.auth = nil
		s.err = nil
		s.notice = msg.result.Message
		s.loading = true
		return false, s.loadSnapshot()

	case privateAccessAuthErrMsg:
		s.clearAuthWait()
		s.authWaiting = false
		if s.auth != nil {
			s.notice = "Authentication was not completed. The login URL can be retried."
		}
		s.err = msg.err

	case tea.KeyPressMsg:
		if s.auth != nil {
			switch msg.String() {
			case "q", "esc":
				s.cancelAuthentication()
				s.auth = nil
				s.err = nil
				s.notice = "Authentication wait canceled."
				return false, nil
			case "r":
				if !s.loading {
					s.loading = true
					s.err = nil
					return false, s.loadSnapshot()
				}
			}
			return false, nil
		}

		if s.acting {
			return false, nil
		}

		switch msg.String() {
		case "q", "esc":
			if s.inActions {
				s.inActions = false
				s.selectedID = ""
				s.actionCursor = 0
				s.err = nil
				s.notice = ""
				return false, nil
			}
			s.cancelAuthentication()
			return true, nil

		case "up", "k":
			s.err = nil
			s.notice = ""
			if s.inActions {
				if s.actionCursor > 0 {
					s.actionCursor--
				}
			} else if s.cursor > 0 {
				s.cursor--
			}

		case "down", "j":
			s.err = nil
			s.notice = ""
			if s.inActions {
				if s.actionCursor+1 < len(s.currentActions()) {
					s.actionCursor++
				}
			} else if s.cursor+1 < len(s.snapshot.VPN) {
				s.cursor++
			}

		case "enter":
			s.err = nil
			s.notice = ""
			if s.inActions {
				actions := s.currentActions()
				if s.actionCursor < len(actions) {
					option := actions[s.actionCursor]
					s.acting = true
					return false, s.runAction(s.selectedID, option.action)
				}
				return false, nil
			}

			provider, ok := s.currentProvider()
			if !ok {
				return false, nil
			}
			if !provider.Installed {
				s.notice = provider.Name + " is not installed. nm-hsp does not install providers."
				return false, nil
			}
			s.selectedID = provider.ID
			s.inActions = true
			s.actionCursor = 0

		case "r":
			if !s.loading {
				s.loading = true
				s.err = nil
				s.notice = ""
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

func (s *privateAccessScreen) runAction(id model.VPNProviderID, action model.VPNAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := s.source.VPNAction(ctx, id, action)
		if err != nil {
			return privateAccessActionErrMsg{err: err}
		}
		return privateAccessActionMsg{
			provider: id,
			action:   action,
			result:   result,
		}
	}
}

func actionWaitsForConnection(action model.VPNAction) bool {
	switch action {
	case model.VPNActionActivate, model.VPNActionConnect, model.VPNActionReconnect:
		return true
	default:
		return false
	}
}

func (s *privateAccessScreen) waitForProviderConnection(id model.VPNProviderID) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		var last model.Snapshot
		for {
			snapshot, err := s.source.Snapshot(ctx)
			if err != nil {
				return privateAccessConnectionErrMsg{err: err}
			}
			last = snapshot
			if providerConnectionSettled(snapshot, id) {
				return privateAccessConnectionMsg{snapshot: snapshot, settled: true}
			}

			select {
			case <-ctx.Done():
				return privateAccessConnectionMsg{snapshot: last, settled: false}
			case <-ticker.C:
			}
		}
	}
}

func providerConnectionSettled(snapshot model.Snapshot, id model.VPNProviderID) bool {
	for _, provider := range snapshot.VPN {
		if provider.ID != id {
			continue
		}
		if provider.Connected || provider.StatusError != "" {
			return true
		}
		switch strings.ToLower(strings.TrimSpace(provider.ConnectionState)) {
		case "needslogin", "loginfailed":
			return true
		}
		return false
	}
	return false
}

func (s *privateAccessScreen) startAuthenticationWait() tea.Cmd {
	if s.auth == nil {
		return nil
	}

	s.cancelAuthentication()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.authCancel = cancel
	s.authWaiting = true
	provider := s.auth.provider
	userCode := s.auth.userCode

	return func() tea.Msg {
		result, err := s.source.VPNWaitAuthentication(ctx, provider, userCode)
		if err != nil {
			return privateAccessAuthErrMsg{err: err}
		}
		return privateAccessAuthMsg{provider: provider, result: result}
	}
}

func (s *privateAccessScreen) cancelAuthentication() {
	if s.authCancel != nil {
		s.authCancel()
		s.authCancel = nil
	}
	s.authWaiting = false
}

func (s *privateAccessScreen) clearAuthWait() {
	if s.authCancel != nil {
		s.authCancel()
		s.authCancel = nil
	}
	s.authWaiting = false
}

func (s *privateAccessScreen) currentProvider() (model.VPNProviderState, bool) {
	if len(s.snapshot.VPN) == 0 || s.cursor >= len(s.snapshot.VPN) {
		return model.VPNProviderState{}, false
	}
	return s.snapshot.VPN[s.cursor], true
}

func (s *privateAccessScreen) selectedProvider() (model.VPNProviderState, bool) {
	for _, provider := range s.snapshot.VPN {
		if provider.ID == s.selectedID {
			return provider, true
		}
	}
	return model.VPNProviderState{}, false
}

func (s *privateAccessScreen) currentActions() []privateAccessActionOption {
	provider, ok := s.selectedProvider()
	if !ok || !provider.Installed {
		return nil
	}

	if !provider.ServiceRunning {
		return []privateAccessActionOption{
			{label: "Activate service and connect", action: model.VPNActionActivate},
		}
	}

	if provider.Connected {
		return []privateAccessActionOption{
			{label: "Disconnect (keep service running)", action: model.VPNActionDisconnect},
			{label: "Deactivate service", action: model.VPNActionDeactivate},
		}
	}

	connectLabel := "Connect"
	if strings.EqualFold(provider.ConnectionState, "NeedsLogin") ||
		strings.EqualFold(provider.ConnectionState, "LoginFailed") {
		connectLabel = "Connect / Authenticate"
	}
	return []privateAccessActionOption{
		{label: connectLabel, action: model.VPNActionConnect},
		{label: "Deactivate service", action: model.VPNActionDeactivate},
	}
}

func (s *privateAccessScreen) clampCursors() {
	if len(s.snapshot.VPN) == 0 {
		s.cursor = 0
		s.inActions = false
		s.selectedID = ""
		return
	}
	if s.cursor >= len(s.snapshot.VPN) {
		s.cursor = len(s.snapshot.VPN) - 1
	}

	if s.inActions {
		if _, ok := s.selectedProvider(); !ok {
			s.inActions = false
			s.selectedID = ""
			s.actionCursor = 0
			return
		}
		actions := s.currentActions()
		if len(actions) == 0 {
			s.actionCursor = 0
		} else if s.actionCursor >= len(actions) {
			s.actionCursor = len(actions) - 1
		}
	}
}

func (s *privateAccessScreen) render(width, height int) string {
	if width < 30 {
		width = 30
	}
	contentWidth := width

	if s.auth != nil {
		return s.renderAuthentication(contentWidth)
	}
	if s.inActions {
		return s.renderActions(contentWidth)
	}

	var out strings.Builder
	header := titleStyle.Render("Private Access") + "  " + mutedStyle.Render("Tailscale and NetBird")
	out.WriteString(lipgloss.NewStyle().Width(contentWidth).Padding(0, 1).Render(header))
	out.WriteString("\n")
	out.WriteString(lipgloss.NewStyle().Width(contentWidth).Padding(0, 1).Render(
		mutedStyle.Render("Local lifecycle controls only. Account and policy settings stay with the provider."),
	))

	if s.loading {
		out.WriteString("\n\n  " + warningStyle.Render("Refreshing private access state..."))
	} else {
		out.WriteString("\n")
		if len(s.snapshot.VPN) == 0 {
			out.WriteString("\n")
			out.WriteString(cardStyle.
				Width(cardContentWidth(contentWidth)).
				Padding(1, 2).
				Render("Private access provider state is unavailable."))
		} else {
			for index, provider := range s.snapshot.VPN {
				out.WriteString("\n")
				out.WriteString(renderPrivateAccessProvider(contentWidth, provider, index == s.cursor))
			}
		}
	}

	s.renderMessages(&out)
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Width(contentWidth).Render("↑/↓ or j/k move   Enter actions   r refresh   Esc/q back"))
	return out.String()
}

func (s *privateAccessScreen) renderActions(width int) string {
	var out strings.Builder
	provider, ok := s.selectedProvider()
	if !ok {
		return "Private Access"
	}
	if s.waitingConnection {
		provider.ServiceState = "active"
		provider.ServiceRunning = true
		provider.ConnectionState = "connecting"
		provider.Connected = false
		provider.Addresses = nil
	}

	header := titleStyle.Render("Private Access · " + emptyFallback(provider.Name, string(provider.ID)))
	out.WriteString(lipgloss.NewStyle().Width(width).Padding(0, 1).Render(header))
	out.WriteString("\n\n")
	out.WriteString(renderPrivateAccessProvider(width, provider, false))
	out.WriteString("\n")
	out.WriteString(lipgloss.NewStyle().Width(width).Padding(0, 1).Render(
		mutedStyle.Render("Disconnect keeps the service running. Deactivate service stops it for this boot."),
	))

	actions := s.currentActions()
	for index, option := range actions {
		style := cardStyle
		marker := "  "
		if index == s.actionCursor {
			style = selectedCardStyle
			marker = "› "
		}
		out.WriteString("\n")
		out.WriteString(style.
			Width(cardContentWidth(width)).
			Render(marker + option.label))
	}

	if s.acting {
		out.WriteString("\n  " + warningStyle.Render("Applying action..."))
	}
	s.renderMessages(&out)
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Width(width).Render("↑/↓ or j/k move   Enter select   r refresh   Esc/q back"))
	return out.String()
}

func (s *privateAccessScreen) renderAuthentication(width int) string {
	var out strings.Builder
	providerName := string(s.auth.provider)
	if provider, ok := s.selectedProvider(); ok {
		providerName = emptyFallback(provider.Name, providerName)
	}

	header := titleStyle.Render("Private Access · " + providerName)
	out.WriteString(lipgloss.NewStyle().Width(width).Padding(0, 1).Render(header))
	out.WriteString("\n\n")
	out.WriteString(warningStyle.Render("Authentication required"))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render("Open this provider URL in a browser to finish sign-in:"))
	out.WriteString("\n\n")
	out.WriteString(cardStyle.
		Width(cardContentWidth(width)).
		Padding(1, 2).
		Render(s.auth.url))
	if s.auth.userCode != "" {
		out.WriteString("\n")
		out.WriteString(cardStyle.
			Width(cardContentWidth(width)).
			Render("Verification code: " + s.auth.userCode))
	}

	if s.authWaiting {
		out.WriteString("\n\n  " + warningStyle.Render("Waiting for provider sign-in..."))
	}
	s.renderMessages(&out)
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Width(width).Render("Esc/q cancel authentication wait"))
	return out.String()
}

func (s *privateAccessScreen) renderMessages(out *strings.Builder) {
	if s.err != nil {
		out.WriteString("\n  " + errorStyle.Render(s.err.Error()))
	}
	if s.notice != "" {
		out.WriteString("\n  " + goodStyle.Render(s.notice))
	}
}

func renderPrivateAccessProvider(width int, provider model.VPNProviderState, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}

	name := emptyFallback(provider.Name, string(provider.ID))
	lines := []string{marker + titleStyle.Render(name)}

	if !provider.Installed {
		lines = append(lines, "    "+mutedStyle.Render("Not installed"))
		return style.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n"))
	}

	serviceEnabled := "disabled"
	if provider.ServiceEnabled {
		serviceEnabled = "enabled"
	}
	serviceState := emptyFallback(provider.ServiceState, "unknown")
	serviceLine := fmt.Sprintf("    Service     %s · %s", serviceEnabled, serviceState)
	lines = append(lines, serviceLine)

	connectionLabel := friendlyProviderConnectionState(provider)
	switch {
	case provider.Connected:
		connectionLabel = goodStyle.Render(connectionLabel)
	case provider.ConnectionState == "unavailable":
		connectionLabel = mutedStyle.Render(connectionLabel)
	case provider.ServiceRunning:
		connectionLabel = warningStyle.Render(connectionLabel)
	default:
		connectionLabel = mutedStyle.Render(connectionLabel)
	}
	lines = append(lines, "    Connection  "+connectionLabel)

	if len(provider.Addresses) > 0 {
		lines = append(lines, "    Addresses   "+strings.Join(provider.Addresses, ", "))
	}
	if provider.StatusError != "" {
		lines = append(lines, "    "+errorStyle.Render(provider.StatusError))
	}

	return style.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n"))
}

func friendlyProviderConnectionState(provider model.VPNProviderState) string {
	if provider.Connected {
		return "connected"
	}
	if !provider.ServiceRunning {
		return "disconnected"
	}

	switch strings.ToLower(strings.TrimSpace(provider.ConnectionState)) {
	case "", "unknown", "nostate", "stopped", "idle":
		return "disconnected"
	case "needslogin", "loginfailed":
		return "authentication required"
	case "starting", "connecting":
		return "connecting"
	case "unavailable":
		return "details unavailable"
	default:
		return "disconnected"
	}
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
		case provider.ServiceRunning:
			state = friendlyProviderConnectionState(provider)
		default:
			state = "stopped"
		}
		parts = append(parts, name+" "+state)
	}
	return strings.Join(parts, " · ")
}
