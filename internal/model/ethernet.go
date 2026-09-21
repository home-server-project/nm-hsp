// SPDX-License-Identifier: Apache-2.0

package model

// IPMethod is the supported IP configuration mode for an Ethernet profile.
type IPMethod string

const (
	IPMethodAuto     IPMethod = "auto"
	IPMethodManual   IPMethod = "manual"
	IPMethodDisabled IPMethod = "disabled"
)

// IPProfileConfig is the persistent IPv4 or IPv6 portion of an Ethernet profile.
type IPProfileConfig struct {
	Method    IPMethod
	Addresses []IPAddress
	Gateway   string
	DNS       []string
}

// EthernetProfile is the editable, non-secret subset of an Ethernet
// NetworkManager connection profile.
type EthernetProfile struct {
	ProfilePath   string
	DevicePath    string
	ID            string
	UUID          string
	InterfaceName string
	Autoconnect   bool
	MTU           uint32
	IPv4          IPProfileConfig
	IPv6          IPProfileConfig
}
