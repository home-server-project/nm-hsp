// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/validation"
)

const (
	update2ToDisk             uint32 = 0x1
	addConnection2ToDisk      uint32 = 0x1
	addConnection2BlockAuto   uint32 = 0x20
)

// EthernetProfile reads the editable, non-secret subset of a saved Ethernet
// profile and associates it with the selected NetworkManager device.
func (c *Client) EthernetProfile(ctx context.Context, profilePath, devicePath string) (model.EthernetProfile, error) {
	path := dbus.ObjectPath(profilePath)
	if !path.IsValid() {
		return model.EthernetProfile{}, fmt.Errorf("invalid connection profile path %q", profilePath)
	}

	settings, err := c.connectionSettings(ctx, path)
	if err != nil {
		return model.EthernetProfile{}, err
	}

	interfaceName := ""
	if devicePath != "" {
		deviceObjectPath := dbus.ObjectPath(devicePath)
		if !deviceObjectPath.IsValid() {
			return model.EthernetProfile{}, fmt.Errorf("invalid device path %q", devicePath)
		}
		props, err := c.getAll(ctx, deviceObjectPath, deviceInterface)
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("read Ethernet device: %w", err)
		}
		interfaceName = stringValue(props, "Interface")
	}

	return parseEthernetProfile(settings, profilePath, devicePath, interfaceName)
}

// SaveEthernetProfile persists a supported Ethernet profile. Existing profiles
// are updated in place; profiles without a ProfilePath are created on disk.
// Neither path activates, deactivates, or reapplies the connection.
func (c *Client) SaveEthernetProfile(ctx context.Context, profile model.EthernetProfile) (model.EthernetProfile, error) {
	if profile.ProfilePath == "" {
		return c.CreateEthernetProfile(ctx, profile)
	}
	if err := c.UpdateEthernetProfile(ctx, profile); err != nil {
		return model.EthernetProfile{}, err
	}
	return profile, nil
}

// CreateEthernetProfile creates a new persistent Ethernet profile without
// activating it.
func (c *Client) CreateEthernetProfile(ctx context.Context, profile model.EthernetProfile) (model.EthernetProfile, error) {
	if err := validation.ValidateEthernetProfile(profile); err != nil {
		return model.EthernetProfile{}, err
	}
	if profile.UUID == "" {
		uuid, err := newUUID()
		if err != nil {
			return model.EthernetProfile{}, fmt.Errorf("generate connection UUID: %w", err)
		}
		profile.UUID = uuid
	}
	if profile.ID == "" {
		profile.ID = "Ethernet " + profile.InterfaceName
	}

	settings := newEthernetSettings(profile)
	var path dbus.ObjectPath
	var result map[string]dbus.Variant
	if err := c.call(
		ctx,
		settingsPath,
		settingsInterface+".AddConnection2",
		settings,
		addConnection2ToDisk|addConnection2BlockAuto,
		map[string]dbus.Variant{},
	).Store(&path, &result); err != nil {
		return model.EthernetProfile{}, fmt.Errorf("create Ethernet profile: %w", err)
	}
	profile.ProfilePath = string(path)
	return profile, nil
}

// UpdateEthernetProfile persists supported Ethernet settings to disk. It does
// not activate, deactivate, or reapply the connection.
func (c *Client) UpdateEthernetProfile(ctx context.Context, profile model.EthernetProfile) error {
	if err := validation.ValidateEthernetProfile(profile); err != nil {
		return err
	}

	path := dbus.ObjectPath(profile.ProfilePath)
	if !path.IsValid() {
		return fmt.Errorf("invalid connection profile path %q", profile.ProfilePath)
	}

	// Re-read immediately before the write so unrelated settings are preserved
	// and a stale form cannot accidentally overwrite a different profile.
	settings, err := c.connectionSettings(ctx, path)
	if err != nil {
		return err
	}

	current, err := parseEthernetProfile(
		settings,
		profile.ProfilePath,
		profile.DevicePath,
		profile.InterfaceName,
	)
	if err != nil {
		return err
	}
	if profile.UUID != "" && current.UUID != profile.UUID {
		return fmt.Errorf("connection profile changed while it was being edited")
	}

	patchEthernetSettings(settings, profile)

	var result map[string]dbus.Variant
	if err := c.call(
		ctx,
		path,
		connectionInterface+".Update2",
		settings,
		update2ToDisk,
		map[string]dbus.Variant{},
	).Store(&result); err != nil {
		return fmt.Errorf("save Ethernet profile: %w", err)
	}

	return nil
}

func (c *Client) connectionSettings(ctx context.Context, path dbus.ObjectPath) (map[string]map[string]dbus.Variant, error) {
	var settings map[string]map[string]dbus.Variant
	if err := c.call(ctx, path, connectionInterface+".GetSettings").Store(&settings); err != nil {
		return nil, fmt.Errorf("read connection profile %s: %w", path, err)
	}
	return settings, nil
}

func parseEthernetProfile(
	settings map[string]map[string]dbus.Variant,
	profilePath string,
	devicePath string,
	deviceInterfaceName string,
) (model.EthernetProfile, error) {
	connection := settings["connection"]
	if stringValue(connection, "type") != "802-3-ethernet" {
		return model.EthernetProfile{}, fmt.Errorf("profile is not an Ethernet connection")
	}
	if _, ok := settings["802-1x"]; ok {
		return model.EthernetProfile{}, fmt.Errorf("802.1X Ethernet profiles are not supported in this checkpoint")
	}

	interfaceName := stringValue(connection, "interface-name")
	if interfaceName == "" {
		interfaceName = deviceInterfaceName
	}

	ipv4, err := parsePersistentIPConfig(settings["ipv4"], false)
	if err != nil {
		return model.EthernetProfile{}, err
	}
	ipv6, err := parsePersistentIPConfig(settings["ipv6"], true)
	if err != nil {
		return model.EthernetProfile{}, err
	}

	return model.EthernetProfile{
		ProfilePath:   profilePath,
		DevicePath:    devicePath,
		ID:            stringValue(connection, "id"),
		UUID:          stringValue(connection, "uuid"),
		InterfaceName: interfaceName,
		Autoconnect:   boolValueDefault(connection, "autoconnect", true),
		MTU:           uint32Value(settings["802-3-ethernet"], "mtu"),
		IPv4:          ipv4,
		IPv6:          ipv6,
	}, nil
}

func parsePersistentIPConfig(values map[string]dbus.Variant, ipv6 bool) (model.IPProfileConfig, error) {
	method := model.IPMethod(stringValue(values, "method"))
	switch method {
	case model.IPMethodAuto, model.IPMethodManual, model.IPMethodDisabled:
	default:
		return model.IPProfileConfig{}, fmt.Errorf(
			"unsupported %s method %q",
			ipFamilyName(ipv6),
			method,
		)
	}

	config := model.IPProfileConfig{
		Method:  method,
		Gateway: stringValue(values, "gateway"),
		DNS:     stringSliceValue(values, "dns-data"),
	}

	if variant, ok := values["address-data"]; ok {
		if addressData, ok := variant.Value().([]map[string]dbus.Variant); ok {
			for _, entry := range addressData {
				address := stringValue(entry, "address")
				if address == "" {
					continue
				}
				config.Addresses = append(config.Addresses, model.IPAddress{
					Address: address,
					Prefix:  uint32Value(entry, "prefix"),
				})
			}
		}
	}

	return config, nil
}

func patchEthernetSettings(settings map[string]map[string]dbus.Variant, profile model.EthernetProfile) {
	connection := ensureSetting(settings, "connection")
	connection["autoconnect"] = dbus.MakeVariant(profile.Autoconnect)

	ethernet := ensureSetting(settings, "802-3-ethernet")
	ethernet["mtu"] = dbus.MakeVariant(profile.MTU)

	patchPersistentIPConfig(ensureSetting(settings, "ipv4"), profile.IPv4)
	patchPersistentIPConfig(ensureSetting(settings, "ipv6"), profile.IPv6)
}

func patchPersistentIPConfig(values map[string]dbus.Variant, config model.IPProfileConfig) {
	values["method"] = dbus.MakeVariant(string(config.Method))

	// The legacy keys take precedence over address-data/gateway on some
	// NetworkManager versions, so never send both representations.
	delete(values, "addresses")
	delete(values, "dns")

	switch config.Method {
	case model.IPMethodAuto:
		delete(values, "address-data")
		delete(values, "gateway")
		setDNSData(values, config.DNS)
		values["ignore-auto-dns"] = dbus.MakeVariant(len(config.DNS) > 0)

	case model.IPMethodManual:
		addressData := make([]map[string]dbus.Variant, 0, len(config.Addresses))
		for _, address := range config.Addresses {
			addressData = append(addressData, map[string]dbus.Variant{
				"address": dbus.MakeVariant(address.Address),
				"prefix":  dbus.MakeVariant(address.Prefix),
			})
		}
		values["address-data"] = dbus.MakeVariant(addressData)
		setOptionalString(values, "gateway", config.Gateway)
		setDNSData(values, config.DNS)
		delete(values, "ignore-auto-dns")

	case model.IPMethodDisabled:
		delete(values, "address-data")
		delete(values, "gateway")
		delete(values, "dns-data")
		delete(values, "ignore-auto-dns")
		delete(values, "routes")
		delete(values, "route-data")
	}
}

func ensureSetting(settings map[string]map[string]dbus.Variant, name string) map[string]dbus.Variant {
	values := settings[name]
	if values == nil {
		values = make(map[string]dbus.Variant)
		settings[name] = values
	}
	return values
}

func setOptionalString(values map[string]dbus.Variant, key, value string) {
	if value == "" {
		delete(values, key)
		return
	}
	values[key] = dbus.MakeVariant(value)
}

func setDNSData(values map[string]dbus.Variant, dns []string) {
	if len(dns) == 0 {
		delete(values, "dns-data")
		return
	}
	values["dns-data"] = dbus.MakeVariant(dns)
}

func stringSliceValue(values map[string]dbus.Variant, key string) []string {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().([]string); ok {
			return append([]string(nil), value...)
		}
	}
	return nil
}

func ipFamilyName(ipv6 bool) string {
	if ipv6 {
		return "IPv6"
	}
	return "IPv4"
}


func newEthernetSettings(profile model.EthernetProfile) map[string]map[string]dbus.Variant {
	settings := map[string]map[string]dbus.Variant{
		"connection": {
			"id":             dbus.MakeVariant(profile.ID),
			"uuid":           dbus.MakeVariant(profile.UUID),
			"type":           dbus.MakeVariant("802-3-ethernet"),
			"interface-name": dbus.MakeVariant(profile.InterfaceName),
		},
	}
	patchEthernetSettings(settings, profile)
	return settings
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	), nil
}
