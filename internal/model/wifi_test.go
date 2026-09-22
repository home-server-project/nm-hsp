// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/security"
)

func TestWiFiConnectRequestFormattingDoesNotLeakPassword(t *testing.T) {
	request := WiFiConnectRequest{
		SSID:          "Home Wi-Fi",
		KeyManagement: "wpa-psk",
		Password:      security.NewSecret("correct-horse"),
	}
	defer request.Password.Clear()

	for _, format := range []string{"%v", "%+v", "%#v"} {
		output := fmt.Sprintf(format, request)
		if strings.Contains(output, "correct-horse") {
			t.Fatalf("format %q leaked Wi-Fi password: %q", format, output)
		}
	}
}
