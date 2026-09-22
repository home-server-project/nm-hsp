// SPDX-License-Identifier: Apache-2.0

package model

// VPNProviderID is the stable machine-facing identifier for a supported
// private-access provider.
type VPNProviderID string

const (
	VPNProviderTailscale VPNProviderID = "tailscale"
	VPNProviderNetBird   VPNProviderID = "netbird"
)

// VPNAction is a small, provider-neutral local lifecycle operation.
type VPNAction string

const (
	VPNActionActivate   VPNAction = "activate"
	VPNActionConnect    VPNAction = "connect"
	VPNActionReconnect  VPNAction = "reconnect"
	VPNActionDisconnect VPNAction = "disconnect"
	VPNActionDeactivate VPNAction = "deactivate"
)

// VPNActionResult contains only user-facing, ephemeral action information.
// Authentication URLs and codes are intentionally never part of Snapshot.
type VPNActionResult struct {
	Message      string
	AuthURL      string
	UserCode     string
	AwaitingAuth bool
}

// VPNProviderState is a point-in-time, read-only view of one supported
// private-access provider. It intentionally contains no credentials or
// provider configuration.
type VPNProviderState struct {
	ID              VPNProviderID
	Name            string
	Installed       bool
	ServiceUnit     string
	ServiceEnabled  bool
	ServiceState    string
	ServiceRunning  bool
	ConnectionState string
	Connected       bool
	Addresses       []string
	StatusError     string `json:",omitempty"`
}
