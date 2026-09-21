// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"strings"

	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/security"
)

// ValidateWiFiConnectRequest validates a new Wi-Fi connection before it is
// passed to NetworkManager.
func ValidateWiFiConnectRequest(request model.WiFiConnectRequest) error {
	if strings.TrimSpace(request.DevicePath) == "" {
		return fmt.Errorf("Wi-Fi device path is required")
	}
	if err := ValidateSSID(request.SSID); err != nil {
		return err
	}

	switch request.KeyManagement {
	case "":
		if !request.Password.Empty() {
			return fmt.Errorf("open Wi-Fi must not contain a password")
		}
	case "owe":
		if !request.Password.Empty() {
			return fmt.Errorf("OWE Wi-Fi must not contain a password")
		}
	case "wpa-psk":
		if !validWPAPSK(request.Password) {
			return fmt.Errorf("WPA/WPA2 password must be 8-63 characters or 64 hexadecimal characters")
		}
	case "sae":
		if request.Password.Empty() {
			return fmt.Errorf("WPA3 password is required")
		}
	default:
		return fmt.Errorf("unsupported Wi-Fi security method %q", request.KeyManagement)
	}

	return nil
}

// ValidateSSID validates the byte-length requirements of an 802.11 SSID.
func ValidateSSID(ssid string) error {
	length := len([]byte(ssid))
	if length == 0 {
		return fmt.Errorf("Wi-Fi network name is required")
	}
	if length > 32 {
		return fmt.Errorf("Wi-Fi network name must be 32 bytes or fewer")
	}
	return nil
}

func validWPAPSK(password security.Secret) bool {
	length := password.Len()
	if length >= 8 && length <= 63 {
		return true
	}
	return length == 64 && password.IsHex()
}
