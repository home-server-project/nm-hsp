// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"context"
	"fmt"
	"sort"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
)

const (
	apFlagPrivacy uint32 = 0x00000001

	apSecKeyMgmtPSK        uint32 = 0x00000100
	apSecKeyMgmt8021X      uint32 = 0x00000200
	apSecKeyMgmtSAE        uint32 = 0x00000400
	apSecKeyMgmtOWE        uint32 = 0x00000800
	apSecKeyMgmtOWETransit uint32 = 0x00001000
	apSecKeyMgmtSuiteB192  uint32 = 0x00002000
)

// WiFiNetworks returns the access points currently known to NetworkManager for
// a Wi-Fi device. Multiple BSSIDs for the same SSID/security pair are collapsed
// to the strongest access point.
func (c *Client) WiFiNetworks(ctx context.Context, devicePath string) ([]model.WiFiNetwork, error) {
	path := dbus.ObjectPath(devicePath)
	if !path.IsValid() {
		return nil, fmt.Errorf("invalid Wi-Fi device path %q", devicePath)
	}

	wireless, err := c.getAll(ctx, path, wirelessInterface)
	if err != nil {
		return nil, fmt.Errorf("read Wi-Fi device: %w", err)
	}
	activeAP := objectPathValue(wireless, "ActiveAccessPoint")

	var apPaths []dbus.ObjectPath
	if err := c.call(ctx, path, wirelessInterface+".GetAllAccessPoints").Store(&apPaths); err != nil {
		return nil, fmt.Errorf("list Wi-Fi access points: %w", err)
	}

	profiles, _, err := c.readProfiles(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for _, profile := range profiles {
		if profile.Type == "802-11-wireless" && profile.SSID != "" {
			known[profile.SSID] = true
		}
	}

	byNetwork := make(map[string]model.WiFiNetwork)
	for _, apPath := range apPaths {
		props, err := c.getAll(ctx, apPath, accessPointInterface)
		if err != nil {
			return nil, fmt.Errorf("read Wi-Fi access point %s: %w", apPath, err)
		}

		ssid := ssidValue(props, "Ssid")
		flags := uint32Value(props, "Flags")
		wpaFlags := uint32Value(props, "WpaFlags")
		rsnFlags := uint32Value(props, "RsnFlags")
		security, keyManagement := classifyWiFiSecurity(flags, wpaFlags, rsnFlags)

		network := model.WiFiNetwork{
			ObjectPath:     string(apPath),
			SSID:           ssid,
			BSSID:          stringValue(props, "HwAddress"),
			Strength:       byteValue(props, "Strength"),
			FrequencyMHz:   uint32Value(props, "Frequency"),
			MaxBitrateKbps: uint32Value(props, "MaxBitrate"),
			Security:       security,
			KeyManagement:  keyManagement,
			Hidden:         ssid == "",
			Known:          known[ssid],
			Active:         validObjectPath(activeAP) && apPath == activeAP,
		}

		key := ssid + "\x00" + string(security)
		if ssid == "" {
			key = string(apPath)
		}
		current, ok := byNetwork[key]
		if !ok || network.Active || (!current.Active && network.Strength > current.Strength) {
			byNetwork[key] = network
		}
	}

	networks := make([]model.WiFiNetwork, 0, len(byNetwork))
	for _, network := range byNetwork {
		networks = append(networks, network)
	}
	sort.SliceStable(networks, func(i, j int) bool {
		if networks[i].Active != networks[j].Active {
			return networks[i].Active
		}
		if networks[i].Known != networks[j].Known {
			return networks[i].Known
		}
		if networks[i].Strength != networks[j].Strength {
			return networks[i].Strength > networks[j].Strength
		}
		return networks[i].SSID < networks[j].SSID
	})
	return networks, nil
}

// RequestWiFiScan asks NetworkManager for a fresh scan. Results are retrieved
// separately with WiFiNetworks after NetworkManager updates its AP list.
func (c *Client) RequestWiFiScan(ctx context.Context, devicePath string) error {
	path := dbus.ObjectPath(devicePath)
	if !path.IsValid() {
		return fmt.Errorf("invalid Wi-Fi device path %q", devicePath)
	}
	if err := c.call(
		ctx,
		path,
		wirelessInterface+".RequestScan",
		map[string]dbus.Variant{},
	).Err; err != nil {
		return fmt.Errorf("request Wi-Fi scan: %w", err)
	}
	return nil
}

func classifyWiFiSecurity(flags, wpaFlags, rsnFlags uint32) (model.WiFiSecurity, string) {
	combined := wpaFlags | rsnFlags

	if combined&(apSecKeyMgmt8021X|apSecKeyMgmtSuiteB192) != 0 {
		return model.WiFiSecurityEnterprise, "wpa-eap"
	}
	if rsnFlags&apSecKeyMgmtSAE != 0 && combined&apSecKeyMgmtPSK == 0 {
		return model.WiFiSecurityWPA3Personal, "sae"
	}
	if combined&apSecKeyMgmtPSK != 0 {
		return model.WiFiSecurityPersonal, "wpa-psk"
	}
	if rsnFlags&(apSecKeyMgmtOWE|apSecKeyMgmtOWETransit) != 0 {
		return model.WiFiSecurityOWE, "owe"
	}
	if flags&apFlagPrivacy != 0 {
		return model.WiFiSecurityWEP, "none"
	}
	if wpaFlags == 0 && rsnFlags == 0 {
		return model.WiFiSecurityOpen, ""
	}
	return model.WiFiSecurityUnknown, ""
}
