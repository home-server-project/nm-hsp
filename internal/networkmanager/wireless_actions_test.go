// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"errors"
	"strings"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/security"
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
		Password:      security.NewSecret("correct-horse"),
		Autoconnect:   true,
	}
	defer request.Password.Clear()

	settings := newWiFiSettings(request, "11111111-2222-4333-8444-555555555555")
	wirelessSecurity := settings["802-11-wireless-security"]
	if got := stringValue(wirelessSecurity, "key-mgmt"); got != "wpa-psk" {
		t.Fatalf("key-mgmt = %q, want wpa-psk", got)
	}
	if got := stringValue(wirelessSecurity, "psk"); got != "correct-horse" {
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

func TestSanitizeWiFiConnectErrorRedactsPassword(t *testing.T) {
	password := security.NewSecret("correct-horse")
	defer password.Clear()

	err := sanitizeWiFiConnectError(errors.New("activation rejected password correct-horse"), password)
	if err == nil {
		t.Fatal("expected sanitized error")
	}
	if strings.Contains(err.Error(), "correct-horse") {
		t.Fatalf("sanitized D-Bus error leaked password: %q", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("sanitized D-Bus error missing redaction marker: %q", err)
	}
}
