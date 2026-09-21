// SPDX-License-Identifier: Apache-2.0

package model

// DeviceKind is the user-facing class of a NetworkManager device.
type DeviceKind string

const (
	DeviceKindEthernet DeviceKind = "ethernet"
	DeviceKindWiFi     DeviceKind = "wifi"
	DeviceKindOther    DeviceKind = "other"
)

// IPAddress is an address and prefix assigned to a device.
type IPAddress struct {
	Address string
	Prefix  uint32
}

// IPConfig is the effective runtime IP configuration reported by NetworkManager.
type IPConfig struct {
	Addresses []IPAddress
	Gateway   string
	DNS       []string
}

// ConnectionProfile contains non-secret NetworkManager profile metadata.
type ConnectionProfile struct {
	ObjectPath          string
	ID                  string
	UUID                string
	Type                string
	InterfaceName       string
	Autoconnect         bool
	AutoconnectPriority int32
	SSID                string
	Hidden              bool
	KeyManagement       string
}

// WirelessState contains runtime information for a Wi-Fi device.
type WirelessState struct {
	SSID        string
	Signal      uint8
	BitrateKbps uint32
}

// WiFiSecurity is the friendly security classification shown in the UI.
type WiFiSecurity string

const (
	WiFiSecurityOpen         WiFiSecurity = "Open"
	WiFiSecurityOWE          WiFiSecurity = "Enhanced Open (OWE)"
	WiFiSecurityPersonal     WiFiSecurity = "WPA/WPA2 Personal"
	WiFiSecurityWPA3Personal WiFiSecurity = "WPA3 Personal"
	WiFiSecurityEnterprise   WiFiSecurity = "Enterprise"
	WiFiSecurityWEP          WiFiSecurity = "Legacy WEP"
	WiFiSecurityUnknown      WiFiSecurity = "Unknown"
)

// WiFiNetwork is one scanned access point normalized for the UI.
type WiFiNetwork struct {
	ObjectPath     string
	SSID           string
	BSSID          string
	Strength       uint8
	FrequencyMHz   uint32
	MaxBitrateKbps uint32
	Security       WiFiSecurity
	KeyManagement  string
	Hidden         bool
	Known          bool
	Active         bool
}

// Device is the normalized application-facing view of a NetworkManager device.
type Device struct {
	ObjectPath            string
	Interface             string
	IPInterface           string
	Kind                  DeviceKind
	DeviceType            uint32
	State                 uint32
	Managed               bool
	HardwareAddress       string
	MTU                   uint32
	Carrier               *bool
	SpeedMbps             uint32
	IPv4                  IPConfig
	IPv6                  IPConfig
	ActiveConnection      *ConnectionProfile
	ActiveConnectionPath  string
	AvailableProfileUUIDs []string
	Wireless              *WirelessState
}

// Snapshot is a point-in-time read-only view of NetworkManager.
type Snapshot struct {
	Version           string
	State             uint32
	Connectivity      uint32
	NetworkingEnabled bool
	WirelessEnabled   bool
	Devices           []Device
	Profiles          []ConnectionProfile
}

// DeviceKindFromNM maps NetworkManager's NMDeviceType values that nm-hsp
// currently presents as first-class devices.
func DeviceKindFromNM(deviceType uint32) DeviceKind {
	switch deviceType {
	case 1:
		return DeviceKindEthernet
	case 2:
		return DeviceKindWiFi
	default:
		return DeviceKindOther
	}
}

// DeviceStateName returns a stable, user-facing state name for NMDeviceState.
func DeviceStateName(state uint32) string {
	switch state {
	case 10:
		return "unmanaged"
	case 20:
		return "unavailable"
	case 30:
		return "disconnected"
	case 40:
		return "prepare"
	case 50:
		return "config"
	case 60:
		return "need-auth"
	case 70:
		return "ip-config"
	case 80:
		return "ip-check"
	case 90:
		return "secondaries"
	case 100:
		return "activated"
	case 110:
		return "deactivating"
	case 120:
		return "failed"
	default:
		return "unknown"
	}
}

// ConnectivityName returns a stable name for NMConnectivityState.
func ConnectivityName(state uint32) string {
	switch state {
	case 1:
		return "none"
	case 2:
		return "portal"
	case 3:
		return "limited"
	case 4:
		return "full"
	default:
		return "unknown"
	}
}
