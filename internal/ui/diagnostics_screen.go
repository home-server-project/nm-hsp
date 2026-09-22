// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	diag "github.com/home-server-project/nm-hsp/internal/diagnostics"
	"github.com/home-server-project/nm-hsp/internal/model"
)

type diagnosticsSource interface {
	Snapshot(context.Context) (model.Snapshot, error)
	ApplyRepair(context.Context, model.RepairAction) error
}

type diagnosticsRefreshMsg struct {
	report model.DiagnosticReport
}

type diagnosticsRefreshErrMsg struct {
	err error
}

type diagnosticsRepairMsg struct {
	label  string
	action model.RepairAction
	err    error
}

type diagnosticsScreen struct {
	source   diagnosticsSource
	report   model.DiagnosticReport
	cursor   int
	loading  bool
	applying bool
	confirm  *model.RepairAction
	notice   string
	err      error
}

func newDiagnosticsScreen(source diagnosticsSource) *diagnosticsScreen {
	return &diagnosticsScreen{
		source:  source,
		loading: true,
	}
}

func (s *diagnosticsScreen) init() tea.Cmd {
	return s.refresh()
}

func (s *diagnosticsScreen) update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case diagnosticsRefreshMsg:
		s.report = msg.report
		s.loading = false
		s.err = nil
		s.clampCursor()
		return false, nil

	case diagnosticsRefreshErrMsg:
		s.loading = false
		s.err = msg.err
		return false, nil

	case diagnosticsRepairMsg:
		s.applying = false
		s.confirm = nil
		if msg.err != nil {
			s.err = msg.err
			return false, nil
		}
		s.notice = msg.label + " completed."
		s.err = nil
		s.loading = true
		return false, s.refreshAfterRepair(msg.action)

	case tea.KeyPressMsg:
		if s.applying {
			return false, nil
		}

		if s.confirm != nil {
			switch msg.String() {
			case "esc", "n", "N":
				s.confirm = nil
				return false, nil
			case "enter", "y", "Y":
				action := *s.confirm
				s.applying = true
				return false, s.apply(action)
			}
			return false, nil
		}

		if s.loading {
			if msg.String() == "esc" {
				return true, nil
			}
			return false, nil
		}

		switch msg.String() {
		case "esc", "q":
			return true, nil
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor+1 < len(s.report.Checks) {
				s.cursor++
			}
		case "enter":
			if check := s.selectedCheck(); check != nil && check.Repair != nil {
				action := *check.Repair
				s.confirm = &action
			}
		case "r":
			s.loading = true
			s.notice = ""
			s.err = nil
			return false, s.refresh()
		}
	}
	return false, nil
}

func (s *diagnosticsScreen) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		snapshot, err := s.source.Snapshot(ctx)
		if err != nil {
			return diagnosticsRefreshErrMsg{err: err}
		}

		probe := model.DiagnosticProbe{}
		if shouldProbeDNS(snapshot) {
			probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
			probe = diag.ProbeDNS(probeCtx)
			probeCancel()
		}

		return diagnosticsRefreshMsg{report: diag.Analyze(snapshot, probe)}
	}
}

func (s *diagnosticsScreen) apply(action model.RepairAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		err := s.source.ApplyRepair(ctx, action)
		return diagnosticsRepairMsg{
			label:  action.Label,
			action: action,
			err:    err,
		}
	}
}

func (s *diagnosticsScreen) refreshAfterRepair(action model.RepairAction) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()

		deadline := time.Now().Add(4 * time.Second)
		for {
			snapshot, err := s.source.Snapshot(ctx)
			if err != nil {
				return diagnosticsRefreshErrMsg{err: err}
			}
			if !diagnosticsNeedsSettle(action, snapshot) || time.Now().After(deadline) {
				probe := model.DiagnosticProbe{}
				if shouldProbeDNS(snapshot) {
					probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
					probe = diag.ProbeDNS(probeCtx)
					probeCancel()
				}
				return diagnosticsRefreshMsg{report: diag.Analyze(snapshot, probe)}
			}
			select {
			case <-ctx.Done():
				return diagnosticsRefreshErrMsg{err: ctx.Err()}
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
}

func diagnosticsNeedsSettle(action model.RepairAction, snapshot model.Snapshot) bool {
	if action.Kind != model.RepairActivateProfile && action.Kind != model.RepairCreateDHCPProfile {
		return false
	}
	for _, device := range snapshot.Devices {
		if action.DevicePath != "" && device.ObjectPath != action.DevicePath {
			continue
		}
		if action.InterfaceName != "" && device.Interface != action.InterfaceName {
			continue
		}
		if device.ActiveConnection == nil {
			return true
		}
		if device.ActiveConnection.IPv4Method == "auto" && len(device.IPv4.Addresses) == 0 {
			return true
		}
		return false
	}
	return true
}

func (s *diagnosticsScreen) selectedCheck() *model.DiagnosticCheck {
	if s.cursor < 0 || s.cursor >= len(s.report.Checks) {
		return nil
	}
	return &s.report.Checks[s.cursor]
}

func (s *diagnosticsScreen) clampCursor() {
	if len(s.report.Checks) == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= len(s.report.Checks) {
		s.cursor = len(s.report.Checks) - 1
	}
}

func (s *diagnosticsScreen) render(width, height int) string {
	if width < 48 {
		width = 48
	}
	if height <= 0 {
		height = 24
	}

	if s.confirm != nil {
		return s.renderConfirmation(width)
	}

	var out strings.Builder
	out.WriteString(titleStyle.Render("Network health & repair"))
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render("Friendly checks first; automatic repair is offered only when the change is unambiguous."))
	out.WriteString("\n\n")

	okCount, warningCount, problemCount := diagnosticCounts(s.report)
	summary := fmt.Sprintf(
		"OK %d   Warnings %d   Problems %d",
		okCount,
		warningCount,
		problemCount,
	)
	if problemCount > 0 {
		out.WriteString(errorStyle.Render(summary))
	} else if warningCount > 0 {
		out.WriteString(warningStyle.Render(summary))
	} else {
		out.WriteString(goodStyle.Render(summary))
	}
	out.WriteString("\n")

	switch {
	case s.loading && len(s.report.Checks) == 0:
		out.WriteString("\n")
		out.WriteString(warningStyle.Render("Running network checks..."))
	case len(s.report.Checks) == 0:
		out.WriteString("\n")
		out.WriteString(cardStyle.Width(cardContentWidth(width)).Render("No diagnostic results are available."))
	default:
		start, end := s.visibleRange(height)
		for index := start; index < end; index++ {
			out.WriteString("\n")
			out.WriteString(s.renderCheck(width, s.report.Checks[index], index == s.cursor))
		}
		if end-start < len(s.report.Checks) {
			out.WriteString("\n")
			out.WriteString(mutedStyle.Render(fmt.Sprintf(
				"Showing %d-%d of %d checks",
				start+1,
				end,
				len(s.report.Checks),
			)))
		}
	}

	if s.err != nil {
		out.WriteString("\n\n")
		out.WriteString(errorStyle.Render(s.err.Error()))
	}
	if s.notice != "" {
		out.WriteString("\n\n")
		out.WriteString(goodStyle.Render(s.notice))
	}
	if s.applying {
		out.WriteString("\n\n")
		out.WriteString(warningStyle.Render("Applying repair..."))
	}

	out.WriteString("\n\n")
	out.WriteString(helpStyle.Render("↑/↓ navigate   Enter review repair   r rerun checks   Esc back"))
	return out.String()
}

func (s *diagnosticsScreen) renderCheck(width int, check model.DiagnosticCheck, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}

	scope := "Overall"
	if check.Interface != "" {
		scope = check.Interface
	}
	status := diagnosticStatusLabel(check.Status)
	header := fmt.Sprintf("%s%s  %s · %s", marker, status, scope, check.Title)
	body := header + "\n    " + check.Detail
	if check.Repair != nil {
		body += "\n    " + warningStyle.Render("Repair available: "+check.Repair.Label+" — press Enter to review")
	}
	return style.Width(cardContentWidth(width)).Render(body)
}

func (s *diagnosticsScreen) renderConfirmation(width int) string {
	action := s.confirm
	var out strings.Builder
	out.WriteString(titleStyle.Render("Confirm repair"))
	out.WriteString("\n\n")
	out.WriteString(warningStyle.Render(action.Label))
	out.WriteString("\n")
	out.WriteString(action.Description)
	out.WriteString("\n\n")

	if action.InterfaceName != "" {
		out.WriteString("Adapter: " + action.InterfaceName + "\n")
	}
	if action.ProfileID != "" {
		out.WriteString("Profile: " + action.ProfileID + "\n")
	}
	out.WriteString("Before: " + emptyFallback(action.Before, "unchanged") + "\n")
	out.WriteString("After:  " + emptyFallback(action.After, "unchanged") + "\n\n")
	out.WriteString(mutedStyle.Render("Only the change shown above will be requested. Network state is re-checked before the repair is applied."))
	out.WriteString("\n\n")
	if s.applying {
		out.WriteString(warningStyle.Render("Applying repair..."))
	} else {
		out.WriteString(helpStyle.Render("Enter / y apply   Esc / n cancel"))
	}
	return cardStyle.Width(cardContentWidth(width)).Render(out.String())
}

func (s *diagnosticsScreen) visibleRange(height int) (int, int) {
	total := len(s.report.Checks)
	if total == 0 {
		return 0, 0
	}

	maxVisible := (height - 9) / 4
	if maxVisible < 1 {
		maxVisible = 1
	}
	if maxVisible > total {
		maxVisible = total
	}

	start := s.cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	if start+maxVisible > total {
		start = total - maxVisible
	}
	return start, start + maxVisible
}

func diagnosticCounts(report model.DiagnosticReport) (okCount, warningCount, problemCount int) {
	for _, check := range report.Checks {
		switch check.Status {
		case model.DiagnosticOK:
			okCount++
		case model.DiagnosticWarning:
			warningCount++
		case model.DiagnosticProblem:
			problemCount++
		}
	}
	return
}

func diagnosticStatusLabel(status model.DiagnosticStatus) string {
	switch status {
	case model.DiagnosticOK:
		return goodStyle.Render("[OK]")
	case model.DiagnosticWarning:
		return warningStyle.Render("[Warning]")
	case model.DiagnosticProblem:
		return errorStyle.Render("[Problem]")
	default:
		return mutedStyle.Render("[Info]")
	}
}

func shouldProbeDNS(snapshot model.Snapshot) bool {
	for _, device := range snapshot.Devices {
		if device.Kind != model.DeviceKindEthernet && device.Kind != model.DeviceKindWiFi {
			continue
		}
		hasAddress := len(device.IPv4.Addresses) > 0 || len(device.IPv6.Addresses) > 0
		hasDNS := len(device.IPv4.DNS) > 0 || len(device.IPv6.DNS) > 0
		if hasAddress && hasDNS {
			return true
		}
	}
	return false
}
