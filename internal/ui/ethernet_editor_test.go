// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestEthernetEditorBuildsManualProfile(t *testing.T) {
	editor := newEthernetEditor(model.EthernetProfile{
		ProfilePath:   "/org/freedesktop/NetworkManager/Settings/1",
		DevicePath:    "/org/freedesktop/NetworkManager/Devices/2",
		ID:            "Wired connection 1",
		UUID:          "11111111-2222-3333-4444-555555555555",
		InterfaceName: "enp2s0",
		Autoconnect:   true,
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	})
	editor.ipv4Method = model.IPMethodManual
	editor.ipv4Addresses = "192.168.0.59/24"
	editor.ipv4Gateway = "192.168.0.1"
	editor.ipv4DNS = "192.168.0.1, 1.1.1.1"
	editor.ipv6Method = model.IPMethodDisabled
	editor.mtu = "1500"

	profile, err := editor.buildProfile()
	if err != nil {
		t.Fatalf("buildProfile() error = %v", err)
	}
	if profile.IPv4.Method != model.IPMethodManual {
		t.Fatalf("IPv4 method = %q", profile.IPv4.Method)
	}
	if len(profile.IPv4.Addresses) != 1 || profile.IPv4.Addresses[0].Address != "192.168.0.59" {
		t.Fatalf("IPv4 addresses = %#v", profile.IPv4.Addresses)
	}
	if profile.IPv6.Method != model.IPMethodDisabled {
		t.Fatalf("IPv6 method = %q", profile.IPv6.Method)
	}
	if profile.MTU != 1500 {
		t.Fatalf("MTU = %d", profile.MTU)
	}
}

func TestEthernetEditorRejectsBadGateway(t *testing.T) {
	editor := newEthernetEditor(model.EthernetProfile{
		ProfilePath:   "/profile",
		DevicePath:    "/device",
		InterfaceName: "enp2s0",
		IPv4:          model.IPProfileConfig{Method: model.IPMethodManual},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodDisabled},
	})
	editor.ipv4Addresses = "192.168.0.59/24"
	editor.ipv4Gateway = "2001:db8::1"

	if _, err := editor.buildProfile(); err == nil {
		t.Fatal("expected wrong-family gateway to fail")
	}
}

func TestEthernetEditorCyclesMethodAndHidesManualFields(t *testing.T) {
	editor := newEthernetEditor(model.EthernetProfile{
		ProfilePath:   "/profile",
		DevicePath:    "/device",
		InterfaceName: "enp2s0",
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	})

	editor.field = fieldIPv4Method
	editor.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if editor.ipv4Method != model.IPMethodManual {
		t.Fatalf("IPv4 method = %q, want manual", editor.ipv4Method)
	}
	if !editor.fieldVisible(fieldIPv4Addresses) {
		t.Fatal("manual IPv4 address field should be visible")
	}

	editor.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if editor.ipv4Method != model.IPMethodDisabled {
		t.Fatalf("IPv4 method = %q, want disabled", editor.ipv4Method)
	}
	if editor.fieldVisible(fieldIPv4Addresses) {
		t.Fatal("disabled IPv4 address field should be hidden")
	}
}

func TestRenderTextCursor(t *testing.T) {
	if got := renderTextCursor("1500", 2); got != "15▏00" {
		t.Fatalf("renderTextCursor() = %q", got)
	}
}
