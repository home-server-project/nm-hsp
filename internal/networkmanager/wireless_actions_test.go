// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestNewWiFiSettingsOpenNetwork(t *testing.T) {
	request := model.WiFiConnectRequest{
		SSID:                "Guest",
		Autoconnect:         true,
		AutoconnectPriority: 5,
	}

	settings := newWiFiSettings(request, "11111111-2222-4333-8444-555555555555")
	if got := ssidValue(settings["802-11-wireless"], "ssid"); got != "Guest" {
		t.Fatalf("SSID = %q, want Guest", got)
	}
	if _, ok := settings["802-11-wireless-security"]; ok {
		t.Fatal("open network should not have a wireless-security setting")
	}
	if got := int32Value(settings["connection"], "autoconnect-priority"); got != 5 {
		t.Fatalf("autoconnect priority = %d, want 5", got)
	}
}

func TestNewWiFiSettingsPersonalNetwork(t *testing.T) {
	request := model.WiFiConnectRequest{
		SSID:          "Home Wi-Fi",
		KeyManagement: "wpa-psk",
		Password:      "correct-horse",
		Autoconnect:   true,
	}

	settings := newWiFiSettings(request, "11111111-2222-4333-8444-555555555555")
	security := settings["802-11-wireless-security"]
	if got := stringValue(security, "key-mgmt"); got != "wpa-psk" {
		t.Fatalf("key-mgmt = %q, want wpa-psk", got)
	}
	if got := stringValue(security, "psk"); got != "correct-horse" {
		t.Fatal("new personal profile did not carry the provided password to NetworkManager settings")
	}
}

func TestNewWiFiSettingsHiddenNetwork(t *testing.T) {
	request := model.WiFiConnectRequest{
		SSID:          "HiddenNet",
		KeyManagement: "owe",
		Hidden:        true,
		Autoconnect:   true,
	}

	settings := newWiFiSettings(request, "11111111-2222-4333-8444-555555555555")
	if !boolValue(settings["802-11-wireless"], "hidden") {
		t.Fatal("hidden network should set the hidden property")
	}
	if got := stringValue(settings["802-11-wireless-security"], "key-mgmt"); got != "owe" {
		t.Fatalf("key-mgmt = %q, want owe", got)
	}
}
