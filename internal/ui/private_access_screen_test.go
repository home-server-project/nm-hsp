// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestPrivateAccessScreenRendersProviderState(t *testing.T) {
	snapshot := sampleSnapshot()
	snapshot.VPN = []model.VPNProviderState{
		{
			ID:              model.VPNProviderTailscale,
			Name:            "Tailscale",
			Installed:       true,
			ServiceEnabled:  true,
			ServiceState:    "active",
			ServiceRunning:  true,
			ConnectionState: "Running",
			Connected:       true,
			Addresses:       []string{"100.64.0.10", "fd7a:115c:a1e0::10"},
		},
		{
			ID:        model.VPNProviderNetBird,
			Name:      "NetBird",
			Installed: false,
		},
	}

	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)
	output := screen.render(96, 30)
	for _, want := range []string{
		"Private Access",
		"Tailscale",
		"connected",
		"100.64.0.10",
		"NetBird",
		"Not installed",
		"r refresh",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("private access screen missing %q", want)
		}
	}
}

func TestPrivateAccessRefreshUpdatesSnapshot(t *testing.T) {
	initial := sampleSnapshot()
	initial.VPN = []model.VPNProviderState{{
		ID:              model.VPNProviderTailscale,
		Name:            "Tailscale",
		Installed:       true,
		ServiceState:    "active",
		ServiceRunning:  true,
		ConnectionState: "NeedsLogin",
	}}
	refreshed := sampleSnapshot()
	refreshed.VPN = []model.VPNProviderState{{
		ID:              model.VPNProviderTailscale,
		Name:            "Tailscale",
		Installed:       true,
		ServiceEnabled:  true,
		ServiceState:    "active",
		ServiceRunning:  true,
		ConnectionState: "Running",
		Connected:       true,
		Addresses:       []string{"100.64.0.20"},
	}}

	source := &fakeSource{snapshot: refreshed}
	screen := newPrivateAccessScreen(source, initial)
	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: 'r'}))
	if closeScreen || cmd == nil || !screen.loading {
		t.Fatal("r should refresh the private access screen")
	}

	closeScreen, _ = screen.update(cmd())
	if closeScreen || screen.loading {
		t.Fatal("refresh result should keep the screen open and clear loading")
	}
	if len(screen.snapshot.VPN) != 1 || !screen.snapshot.VPN[0].Connected {
		t.Fatalf("private access snapshot was not refreshed: %#v", screen.snapshot.VPN)
	}
}

func TestPrivateAccessEscReturnsToDashboard(t *testing.T) {
	screen := newPrivateAccessScreen(&fakeSource{}, model.Snapshot{})
	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if !closeScreen || cmd != nil {
		t.Fatal("Esc should close the private access screen")
	}
}

func TestPrivateAccessSummary(t *testing.T) {
	snapshot := model.Snapshot{VPN: []model.VPNProviderState{
		{
			ID:              model.VPNProviderTailscale,
			Name:            "Tailscale",
			Installed:       true,
			ServiceState:    "active",
			ServiceRunning:  true,
			ConnectionState: "Running",
			Connected:       true,
		},
		{
			ID:        model.VPNProviderNetBird,
			Name:      "NetBird",
			Installed: false,
		},
	}}

	summary := privateAccessSummary(snapshot)
	if summary != "Tailscale connected · NetBird not installed" {
		t.Fatalf("summary = %q", summary)
	}
}
