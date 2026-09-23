// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func privateAccessTestSnapshot() model.Snapshot {
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
	return snapshot
}

func TestPrivateAccessScreenRendersProviderState(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)
	output := screen.render(96, 30)
	for _, want := range []string{
		"Private Access",
		"Tailscale",
		"connected",
		"100.64.0.10",
		"NetBird",
		"Not installed",
		"Enter actions",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("private access screen missing %q", want)
		}
	}
}

func TestEnterOpensConnectedProviderActions(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)

	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if closeScreen || cmd != nil || !screen.inActions {
		t.Fatal("Enter should open provider actions")
	}
	output := screen.render(96, 30)
	for _, want := range []string{
		"Disconnect (keep service enabled)",
		"Disable service",
		"Disconnect keeps the service enabled",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("action screen missing %q", want)
		}
	}
	if strings.Contains(output, "Reconnect") {
		t.Fatal("connected provider must not offer Reconnect")
	}
}


func TestNoStateRendersAsDisconnected(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	snapshot.VPN[0].Connected = false
	snapshot.VPN[0].ConnectionState = "NoState"
	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)

	output := screen.render(96, 30)
	if !strings.Contains(output, "disconnected") {
		t.Fatalf("NoState should render as disconnected: %q", output)
	}
	if strings.Contains(output, "NoState") {
		t.Fatalf("raw provider state leaked into UI: %q", output)
	}
}

func TestDisconnectedProviderOffersConnect(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	snapshot.VPN[0].Connected = false
	snapshot.VPN[0].ConnectionState = "Stopped"
	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)
	screen.selectedID = model.VPNProviderTailscale
	screen.inActions = true

	actions := screen.currentActions()
	if len(actions) != 2 || actions[0].action != model.VPNActionConnect {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestStoppedProviderOffersActivate(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	snapshot.VPN[0].Connected = false
	snapshot.VPN[0].ServiceRunning = false
	snapshot.VPN[0].ServiceState = "inactive"
	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)
	screen.selectedID = model.VPNProviderTailscale
	screen.inActions = true

	actions := screen.currentActions()
	if len(actions) == 0 || actions[0].action != model.VPNActionActivate {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestUninstalledProviderDoesNotOfferInstall(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	source := &fakeSource{snapshot: snapshot}
	screen := newPrivateAccessScreen(source, snapshot)
	screen.cursor = 1

	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if closeScreen || cmd != nil || screen.inActions {
		t.Fatal("uninstalled provider must not open lifecycle actions")
	}
	if !strings.Contains(screen.notice, "does not install") {
		t.Fatalf("notice = %q", screen.notice)
	}
}

func TestProviderActionUsesSelectedProvider(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	source := &fakeSource{
		snapshot: snapshot,
		vpnActionResult: model.VPNActionResult{
			Message: "disconnected",
		},
	}
	screen := newPrivateAccessScreen(source, snapshot)
	screen.selectedID = model.VPNProviderTailscale
	screen.inActions = true

	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if closeScreen || cmd == nil || !screen.acting {
		t.Fatal("Enter should run selected provider action")
	}
	screen.update(cmd())
	if source.vpnActionProvider != model.VPNProviderTailscale ||
		source.vpnAction != model.VPNActionDisconnect {
		t.Fatalf("provider=%q action=%q", source.vpnActionProvider, source.vpnAction)
	}
}

func TestAuthenticationHandoffStaysInScreenMemory(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	source := &fakeSource{
		snapshot: snapshot,
		vpnActionResult: model.VPNActionResult{
			Message:      "Open provider login",
			AuthURL:      "https://login.example/device",
			UserCode:     "ABCD-EFGH",
			AwaitingAuth: true,
		},
		vpnWaitResult: model.VPNActionResult{
			Message: "authentication complete",
		},
	}
	screen := newPrivateAccessScreen(source, snapshot)
	screen.selectedID = model.VPNProviderTailscale
	screen.inActions = true

	_, actionCmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	authMsg := actionCmd()
	_, waitCmd := screen.update(authMsg)
	if waitCmd == nil || screen.auth == nil || !screen.authWaiting {
		t.Fatal("authentication action should show handoff and start a cancellable wait")
	}

	output := screen.render(96, 30)
	for _, want := range []string{"https://login.example/device", "ABCD-EFGH", "Waiting for provider sign-in"} {
		if !strings.Contains(output, want) {
			t.Fatalf("authentication view missing %q", want)
		}
	}

	waitMsg := waitCmd()
	_, refreshCmd := screen.update(waitMsg)
	if refreshCmd == nil || screen.auth != nil || screen.authWaiting {
		t.Fatal("completed authentication should clear ephemeral handoff and refresh")
	}
}

func TestAuthenticationEscCancelsWaitWithoutClosingScreen(t *testing.T) {
	snapshot := privateAccessTestSnapshot()
	screen := newPrivateAccessScreen(&fakeSource{snapshot: snapshot}, snapshot)
	screen.auth = &privateAccessAuthState{
		provider: model.VPNProviderTailscale,
		url:      "https://login.example/",
	}

	closeScreen, cmd := screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if closeScreen || cmd != nil || screen.auth != nil {
		t.Fatal("Esc during authentication should cancel the wait and return to provider actions")
	}
}

func TestPrivateAccessRefreshUpdatesSnapshot(t *testing.T) {
	initial := privateAccessTestSnapshot()
	refreshed := privateAccessTestSnapshot()
	refreshed.VPN[0].Addresses = []string{"100.64.0.20"}

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
	if screen.snapshot.VPN[0].Addresses[0] != "100.64.0.20" {
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
