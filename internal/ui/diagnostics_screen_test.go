// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func diagnosticRepairFixture() model.RepairAction {
	return model.RepairAction{
		Kind:          model.RepairEnableAutoconnect,
		Label:         "Enable autoconnect",
		Description:   "Change only the autoconnect flag.",
		DevicePath:    "/device/1",
		InterfaceName: "enp2s0",
		ProfilePath:   "/profile/1",
		ProfileID:     "Wired",
		Before:        "autoconnect off",
		After:         "autoconnect on",
	}
}

func TestDiagnosticsRepairRequiresReviewAndSecondConfirmation(t *testing.T) {
	source := &fakeSource{}
	action := diagnosticRepairFixture()
	screen := &diagnosticsScreen{
		source: source,
		report: model.DiagnosticReport{
			Checks: []model.DiagnosticCheck{
				{
					ID:     "autoconnect",
					Status: model.DiagnosticWarning,
					Title:  "Automatic reconnect is disabled",
					Repair: &action,
				},
			},
		},
	}

	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if closeScreen || cmd != nil {
		t.Fatal("first Enter should only open repair review")
	}
	if screen.confirm == nil {
		t.Fatal("repair review was not opened")
	}
	if source.appliedRepair != nil {
		t.Fatal("repair must not apply on the first Enter")
	}

	closeScreen, cmd = screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if closeScreen || cmd == nil {
		t.Fatal("second Enter should return the approved repair command")
	}
	if !screen.applying {
		t.Fatal("screen should show repair as applying")
	}

	msg := cmd()
	if source.appliedRepair == nil {
		t.Fatal("approved repair was not sent to the backend")
	}
	operation, ok := msg.(diagnosticsRepairMsg)
	if !ok || operation.err != nil {
		t.Fatalf("repair command result = %#v", msg)
	}
}

func TestDiagnosticsConfirmationShowsBeforeAfter(t *testing.T) {
	action := diagnosticRepairFixture()
	screen := &diagnosticsScreen{confirm: &action}

	output := screen.render(90, 24)
	for _, want := range []string{
		"Confirm repair",
		"Before: autoconnect off",
		"After:  autoconnect on",
		"Network state is re-checked",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("confirmation render missing %q", want)
		}
	}
}

func TestDiagnosticsNonRepairableCheckDoesNothingOnEnter(t *testing.T) {
	screen := &diagnosticsScreen{
		report: model.DiagnosticReport{
			Checks: []model.DiagnosticCheck{
				{
					ID:     "gateway",
					Status: model.DiagnosticWarning,
					Title:  "No default gateway",
				},
			},
		},
	}

	_, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil || screen.confirm != nil {
		t.Fatal("non-repairable diagnostic must not start a repair")
	}
}

func TestDiagnosticsRenderShowsRepairAvailability(t *testing.T) {
	action := diagnosticRepairFixture()
	screen := &diagnosticsScreen{
		report: model.DiagnosticReport{
			Checks: []model.DiagnosticCheck{
				{
					ID:        "autoconnect",
					Interface: "enp2s0",
					Status:    model.DiagnosticWarning,
					Title:     "Automatic reconnect is disabled",
					Detail:    "Wired will not reconnect after boot.",
					Repair:    &action,
				},
			},
		},
	}

	output := screen.render(90, 24)
	for _, want := range []string{
		"Network health & repair",
		"enp2s0",
		"Automatic reconnect is disabled",
		"Repair available: Enable autoconnect",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("diagnostics render missing %q", want)
		}
	}
}

func TestShouldProbeDNSOnlyWithAddressAndDNS(t *testing.T) {
	snapshot := model.Snapshot{
		Devices: []model.Device{
			{
				Kind: model.DeviceKindEthernet,
				IPv4: model.IPConfig{
					Addresses: []model.IPAddress{{Address: "192.168.0.50", Prefix: 24}},
					DNS:       []string{"192.168.0.1"},
				},
			},
		},
	}
	if !shouldProbeDNS(snapshot) {
		t.Fatal("address plus DNS should enable the DNS probe")
	}
	snapshot.Devices[0].IPv4.DNS = nil
	if shouldProbeDNS(snapshot) {
		t.Fatal("DNS probe should not run without configured DNS servers")
	}
}

func TestDiagnosticsSettleWaitsForDHCPAddress(t *testing.T) {
	action := model.RepairAction{
		Kind:          model.RepairActivateProfile,
		DevicePath:    "/device/1",
		InterfaceName: "enp2s0",
	}
	snapshot := model.Snapshot{
		Devices: []model.Device{
			{
				ObjectPath: "/device/1",
				Interface:  "enp2s0",
				ActiveConnection: &model.ConnectionProfile{
					IPv4Method: "auto",
				},
			},
		},
	}
	if !diagnosticsNeedsSettle(action, snapshot) {
		t.Fatal("activation should settle while DHCP has no IPv4 address")
	}
	snapshot.Devices[0].IPv4.Addresses = []model.IPAddress{{Address: "192.168.0.50", Prefix: 24}}
	if diagnosticsNeedsSettle(action, snapshot) {
		t.Fatal("settle should finish once IPv4 is present")
	}
}
