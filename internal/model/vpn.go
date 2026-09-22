// SPDX-License-Identifier: Apache-2.0

package model

// VPNProviderID is the stable machine-facing identifier for a supported
// private-access provider.
type VPNProviderID string

const (
	VPNProviderTailscale VPNProviderID = "tailscale"
	VPNProviderNetBird   VPNProviderID = "netbird"
)

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
	ConnectionState string
	Connected       bool
	Addresses       []string
	StatusError     string `json:",omitempty"`
}
