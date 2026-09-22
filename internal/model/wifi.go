// SPDX-License-Identifier: Apache-2.0

package model

import "github.com/home-server-project/nm-hsp/internal/security"

// WiFiConnectRequest contains the information needed to create and activate a
// supported Wi-Fi connection. Password is only populated for a new personal
// network connection and must never be logged or persisted by nm-hsp itself.
type WiFiConnectRequest struct {
	DevicePath          string
	AccessPointPath     string
	SSID                string
	KeyManagement       string
	Password            security.Secret
	Hidden              bool
	Autoconnect         bool
	AutoconnectPriority int32
}

// WiFiProfileUpdate contains the editable non-secret metadata for a saved
// Wi-Fi profile.
type WiFiProfileUpdate struct {
	ProfilePath         string
	Autoconnect         bool
	AutoconnectPriority int32
}
