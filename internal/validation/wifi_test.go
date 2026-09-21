// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/security"
)

func TestValidateSSID(t *testing.T) {
	if err := ValidateSSID("Home Wi-Fi"); err != nil {
		t.Fatalf("normal SSID rejected: %v", err)
	}
	if err := ValidateSSID(""); err == nil {
		t.Fatal("empty SSID should fail")
	}
	if err := ValidateSSID(strings.Repeat("x", 33)); err == nil {
		t.Fatal("33-byte SSID should fail")
	}
}

func TestValidateWiFiConnectRequest(t *testing.T) {
	base := model.WiFiConnectRequest{
		DevicePath: "/org/freedesktop/NetworkManager/Devices/3",
		SSID:       "Home Wi-Fi",
	}

	if err := ValidateWiFiConnectRequest(base); err != nil {
		t.Fatalf("open network rejected: %v", err)
	}

	personal := base
	personal.KeyManagement = "wpa-psk"
	personal.Password = security.NewSecret("correct-horse")
	if err := ValidateWiFiConnectRequest(personal); err != nil {
		personal.Password.Clear()
		t.Fatalf("personal network rejected: %v", err)
	}
	personal.Password.Clear()

	personal.Password = security.NewSecret("short")
	if err := ValidateWiFiConnectRequest(personal); err == nil {
		personal.Password.Clear()
		t.Fatal("short WPA PSK should fail")
	}
	personal.Password.Clear()

	enterprise := base
	enterprise.KeyManagement = "wpa-eap"
	if err := ValidateWiFiConnectRequest(enterprise); err == nil {
		t.Fatal("enterprise creation should be unsupported")
	}
}
