// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"errors"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
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

func TestScrubWiFiSettingsSecretDropsTemporaryPSK(t *testing.T) {
	request := model.WiFiConnectRequest{
		SSID:          "Home Wi-Fi",
		KeyManagement: "wpa-psk",
		Password:      security.NewSecret("correct-horse"),
		Autoconnect:   true,
	}
	defer request.Password.Clear()

	settings := newWiFiSettings(request, "11111111-2222-4333-8444-555555555555")
	wirelessSecurity := settings["802-11-wireless-security"]
	if got := stringValue(wirelessSecurity, "psk"); got != "correct-horse" {
		t.Fatalf("precondition failed: psk = %q", got)
	}

	scrubWiFiSettingsSecret(settings)
	if _, ok := wirelessSecurity["psk"]; ok {
		t.Fatal("temporary D-Bus settings must not retain the PSK after use")
	}
}

func TestPatchWiFiProfileMetadataPreservesLegacyIPv6Signatures(t *testing.T) {
	settings := map[string]map[string]dbus.Variant{
		"connection": {
			"type":        dbus.MakeVariant("802-11-wireless"),
			"autoconnect": dbus.MakeVariant(true),
		},
		"ipv6": {
			"addresses": dbus.MakeVariantWithSignature(
				[]any{[]any{[]byte{0x20, 0x01}, uint32(64), []byte{}}},
				dbus.ParseSignatureMust("a(ayuay)"),
			),
			"routes": dbus.MakeVariantWithSignature(
				[]any{[]any{[]byte{}, uint32(0), []byte{}, uint32(0)}},
				dbus.ParseSignatureMust("a(ayuayu)"),
			),
		},
	}

	err := patchWiFiProfileMetadata(settings, model.WiFiProfileUpdate{
		Autoconnect:         false,
		AutoconnectPriority: 7,
	})
	if err != nil {
		t.Fatalf("patchWiFiProfileMetadata() error = %v", err)
	}
	if sig := settings["ipv6"]["addresses"].Signature().String(); sig != "a(ayuay)" {
		t.Fatalf("legacy IPv6 addresses signature changed to %q", sig)
	}
	if sig := settings["ipv6"]["routes"].Signature().String(); sig != "a(ayuayu)" {
		t.Fatalf("legacy IPv6 routes signature changed to %q", sig)
	}
	if boolValue(settings["connection"], "autoconnect") {
		t.Fatal("autoconnect should be false")
	}
	if got := int32Value(settings["connection"], "autoconnect-priority"); got != 7 {
		t.Fatalf("priority = %d, want 7", got)
	}
}

func TestForgetWiFiProfileRejectsNonWiFiSettings(t *testing.T) {
	settings := map[string]map[string]dbus.Variant{
		"connection": {
			"type": dbus.MakeVariant("802-3-ethernet"),
		},
	}
	if stringValue(settings["connection"], "type") == "802-11-wireless" {
		t.Fatal("test precondition failed")
	}
}
