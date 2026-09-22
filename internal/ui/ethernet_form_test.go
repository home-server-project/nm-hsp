// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestEthernetFormBuildsManualProfile(t *testing.T) {
	form := newEthernetForm(model.EthernetProfile{
		ProfilePath:   "/org/freedesktop/NetworkManager/Settings/1",
		DevicePath:    "/org/freedesktop/NetworkManager/Devices/2",
		ID:            "Wired connection 1",
		UUID:          "11111111-2222-3333-4444-555555555555",
		InterfaceName: "enp2s0",
		Autoconnect:   true,
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	})
	form.ipv4Method = model.IPMethodManual
	form.ipv4Addresses.SetValue("192.168.0.59/24")
	form.ipv4Gateway.SetValue("192.168.0.1")
	form.ipv4DNS.SetValue("192.168.0.1, 1.1.1.1")
	form.ipv6Method = model.IPMethodDisabled
	form.mtu.SetValue("1500")

	profile, err := form.buildProfile()
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

func TestEthernetFormRejectsBadGateway(t *testing.T) {
	form := newEthernetForm(model.EthernetProfile{
		ProfilePath:   "/profile",
		DevicePath:    "/device",
		InterfaceName: "enp2s0",
		IPv4:          model.IPProfileConfig{Method: model.IPMethodManual},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodDisabled},
	})
	form.ipv4Addresses.SetValue("192.168.0.59/24")
	form.ipv4Gateway.SetValue("2001:db8::1")

	if _, err := form.buildProfile(); err == nil {
		t.Fatal("expected wrong-family gateway to fail")
	}
}

func TestEthernetFormUsesNormalFieldNavigation(t *testing.T) {
	form := newEthernetForm(model.EthernetProfile{
		ProfilePath:   "/profile",
		DevicePath:    "/device",
		InterfaceName: "enp2s0",
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	})

	if form.field != fieldIPv4Method {
		t.Fatalf("initial field = %d", form.field)
	}

	action, _ := form.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if action != formActionNone || form.field != fieldIPv4DNS {
		t.Fatalf("Enter should advance to the next visible field; action=%d field=%d", action, form.field)
	}

	action, _ = form.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if action != formActionNone || form.field != fieldIPv6Method {
		t.Fatalf("Tab should advance fields; action=%d field=%d", action, form.field)
	}
}

func TestEthernetFormCyclesOptionsWithoutEditingText(t *testing.T) {
	form := newEthernetForm(model.EthernetProfile{
		ProfilePath:   "/profile",
		DevicePath:    "/device",
		InterfaceName: "enp2s0",
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	})

	form.field = fieldIPv4Method
	_, _ = form.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if form.ipv4Method != model.IPMethodManual {
		t.Fatalf("IPv4 method = %q, want manual", form.ipv4Method)
	}
	if !form.fieldVisible(fieldIPv4Addresses) {
		t.Fatal("manual IPv4 address field should be visible")
	}
}

func TestEthernetFormHasSaveAndCancelActions(t *testing.T) {
	form := newEthernetForm(model.EthernetProfile{
		ProfilePath:   "/profile",
		DevicePath:    "/device",
		InterfaceName: "enp2s0",
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	})

	form.field = fieldSave
	action, _ := form.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if action != formActionSave {
		t.Fatalf("Save action = %d", action)
	}

	form.field = fieldCancel
	action, _ = form.handleKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if action != formActionCancel {
		t.Fatalf("Cancel action = %d", action)
	}
}
