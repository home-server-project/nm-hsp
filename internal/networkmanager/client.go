// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
)

const (
	serviceName          = "org.freedesktop.NetworkManager"
	managerPath          = dbus.ObjectPath("/org/freedesktop/NetworkManager")
	settingsPath         = dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings")
	managerInterface     = "org.freedesktop.NetworkManager"
	deviceInterface      = "org.freedesktop.NetworkManager.Device"
	wiredInterface       = "org.freedesktop.NetworkManager.Device.Wired"
	wirelessInterface    = "org.freedesktop.NetworkManager.Device.Wireless"
	accessPointInterface = "org.freedesktop.NetworkManager.AccessPoint"
	activeInterface      = "org.freedesktop.NetworkManager.Connection.Active"
	ip4Interface         = "org.freedesktop.NetworkManager.IP4Config"
	ip6Interface         = "org.freedesktop.NetworkManager.IP6Config"
	settingsInterface    = "org.freedesktop.NetworkManager.Settings"
	connectionInterface  = "org.freedesktop.NetworkManager.Settings.Connection"
	propertiesInterface  = "org.freedesktop.DBus.Properties"
)

// Client is a read-only NetworkManager D-Bus client at this stage of the project.
type Client struct {
	conn *dbus.Conn
}

// NewSystem connects to the system D-Bus where NetworkManager publishes its API.
func NewSystem(ctx context.Context) (*Client, error) {
	conn, err := dbus.ConnectSystemBus(dbus.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("connect to system D-Bus: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Close releases the D-Bus connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Snapshot reads the current NetworkManager state without modifying it.
func (c *Client) Snapshot(ctx context.Context) (model.Snapshot, error) {
	managerProps, err := c.getAll(ctx, managerPath, managerInterface)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("read NetworkManager properties: %w", err)
	}

	profiles, profilesByPath, err := c.readProfiles(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}

	var devicePaths []dbus.ObjectPath
	if err := c.call(ctx, managerPath, managerInterface+".GetDevices").Store(&devicePaths); err != nil {
		return model.Snapshot{}, fmt.Errorf("list NetworkManager devices: %w", err)
	}

	snapshot := model.Snapshot{
		Version:           stringValue(managerProps, "Version"),
		State:             uint32Value(managerProps, "State"),
		Connectivity:      uint32Value(managerProps, "Connectivity"),
		NetworkingEnabled: boolValue(managerProps, "NetworkingEnabled"),
		WirelessEnabled:   boolValue(managerProps, "WirelessEnabled"),
		Profiles:          profiles,
		Devices:           make([]model.Device, 0, len(devicePaths)),
	}

	for _, path := range devicePaths {
		device, err := c.readDevice(ctx, path, profilesByPath)
		if err != nil {
			return model.Snapshot{}, fmt.Errorf("read device %s: %w", path, err)
		}
		snapshot.Devices = append(snapshot.Devices, device)
	}

	sort.Slice(snapshot.Devices, func(i, j int) bool {
		return snapshot.Devices[i].Interface < snapshot.Devices[j].Interface
	})

	return snapshot, nil
}

func (c *Client) readProfiles(ctx context.Context) ([]model.ConnectionProfile, map[dbus.ObjectPath]model.ConnectionProfile, error) {
	var paths []dbus.ObjectPath
	if err := c.call(ctx, settingsPath, settingsInterface+".ListConnections").Store(&paths); err != nil {
		return nil, nil, fmt.Errorf("list NetworkManager connection profiles: %w", err)
	}

	profiles := make([]model.ConnectionProfile, 0, len(paths))
	byPath := make(map[dbus.ObjectPath]model.ConnectionProfile, len(paths))

	for _, path := range paths {
		var settings map[string]map[string]dbus.Variant
		if err := c.call(ctx, path, connectionInterface+".GetSettings").Store(&settings); err != nil {
			return nil, nil, fmt.Errorf("read connection profile %s: %w", path, err)
		}

		connection := settings["connection"]
		profile := model.ConnectionProfile{
			ObjectPath:          string(path),
			ID:                  stringValue(connection, "id"),
			UUID:                stringValue(connection, "uuid"),
			Type:                stringValue(connection, "type"),
			InterfaceName:       stringValue(connection, "interface-name"),
			Autoconnect:         boolValueDefault(connection, "autoconnect", true),
			AutoconnectPriority: int32Value(connection, "autoconnect-priority"),
		}

		profiles = append(profiles, profile)
		byPath[path] = profile
	}

	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].ID == profiles[j].ID {
			return profiles[i].UUID < profiles[j].UUID
		}
		return profiles[i].ID < profiles[j].ID
	})

	return profiles, byPath, nil
}

func (c *Client) readDevice(ctx context.Context, path dbus.ObjectPath, profiles map[dbus.ObjectPath]model.ConnectionProfile) (model.Device, error) {
	props, err := c.getAll(ctx, path, deviceInterface)
	if err != nil {
		return model.Device{}, err
	}

	device := model.Device{
		ObjectPath:      string(path),
		Interface:       stringValue(props, "Interface"),
		IPInterface:     stringValue(props, "IpInterface"),
		DeviceType:      uint32Value(props, "DeviceType"),
		State:           uint32Value(props, "State"),
		Managed:         boolValue(props, "Managed"),
		HardwareAddress: stringValue(props, "HwAddress"),
		MTU:             uint32Value(props, "Mtu"),
	}
	device.Kind = model.DeviceKindFromNM(device.DeviceType)

	if path := objectPathValue(props, "Ip4Config"); validObjectPath(path) {
		ip, err := c.readIPConfig(ctx, path, ip4Interface)
		if err != nil {
			return model.Device{}, fmt.Errorf("read IPv4 configuration: %w", err)
		}
		device.IPv4 = ip
	}

	if path := objectPathValue(props, "Ip6Config"); validObjectPath(path) {
		ip, err := c.readIPConfig(ctx, path, ip6Interface)
		if err != nil {
			return model.Device{}, fmt.Errorf("read IPv6 configuration: %w", err)
		}
		device.IPv6 = ip
	}

	if path := objectPathValue(props, "ActiveConnection"); validObjectPath(path) {
		profile, err := c.readActiveProfile(ctx, path, profiles)
		if err != nil {
			return model.Device{}, err
		}
		device.ActiveConnection = profile
	}

	for _, profilePath := range objectPathSliceValue(props, "AvailableConnections") {
		if profile, ok := profiles[profilePath]; ok && profile.UUID != "" {
			device.AvailableProfileUUIDs = append(device.AvailableProfileUUIDs, profile.UUID)
		}
	}
	sort.Strings(device.AvailableProfileUUIDs)

	switch device.Kind {
	case model.DeviceKindEthernet:
		if wired, err := c.getAll(ctx, path, wiredInterface); err == nil {
			carrier := boolValue(wired, "Carrier")
			device.Carrier = &carrier
			device.SpeedMbps = uint32Value(wired, "Speed")
		}
	case model.DeviceKindWiFi:
		if wireless, err := c.getAll(ctx, path, wirelessInterface); err == nil {
			state := &model.WirelessState{
				BitrateKbps: uint32Value(wireless, "Bitrate"),
			}
			if apPath := objectPathValue(wireless, "ActiveAccessPoint"); validObjectPath(apPath) {
				if ap, err := c.getAll(ctx, apPath, accessPointInterface); err == nil {
					state.SSID = ssidValue(ap, "Ssid")
					state.Signal = byteValue(ap, "Strength")
				}
			}
			device.Wireless = state
		}
	}

	return device, nil
}

func (c *Client) readActiveProfile(ctx context.Context, path dbus.ObjectPath, profiles map[dbus.ObjectPath]model.ConnectionProfile) (*model.ConnectionProfile, error) {
	props, err := c.getAll(ctx, path, activeInterface)
	if err != nil {
		return nil, fmt.Errorf("read active connection: %w", err)
	}

	settingsPath := objectPathValue(props, "Connection")
	if profile, ok := profiles[settingsPath]; ok {
		copy := profile
		return &copy, nil
	}

	profile := model.ConnectionProfile{
		ObjectPath: string(settingsPath),
		ID:         stringValue(props, "Id"),
		UUID:       stringValue(props, "Uuid"),
		Type:       stringValue(props, "Type"),
	}
	return &profile, nil
}

func (c *Client) readIPConfig(ctx context.Context, path dbus.ObjectPath, iface string) (model.IPConfig, error) {
	props, err := c.getAll(ctx, path, iface)
	if err != nil {
		return model.IPConfig{}, err
	}
	return parseIPConfigProperties(props), nil
}

func (c *Client) getAll(ctx context.Context, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var props map[string]dbus.Variant
	if err := c.call(ctx, path, propertiesInterface+".GetAll", iface).Store(&props); err != nil {
		return nil, err
	}
	return props, nil
}

func (c *Client) call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) *dbus.Call {
	return c.conn.Object(serviceName, path).CallWithContext(ctx, method, 0, args...)
}

func parseIPConfigProperties(props map[string]dbus.Variant) model.IPConfig {
	config := model.IPConfig{
		Gateway: stringValue(props, "Gateway"),
	}

	if variant, ok := props["AddressData"]; ok {
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

	if variant, ok := props["NameserverData"]; ok {
		if nameserverData, ok := variant.Value().([]map[string]dbus.Variant); ok {
			for _, entry := range nameserverData {
				if address := stringValue(entry, "address"); address != "" {
					config.DNS = append(config.DNS, address)
				}
			}
		}
	}

	return config
}

func stringValue(values map[string]dbus.Variant, key string) string {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().(string); ok {
			return value
		}
	}
	return ""
}

func boolValue(values map[string]dbus.Variant, key string) bool {
	return boolValueDefault(values, key, false)
}

func boolValueDefault(values map[string]dbus.Variant, key string, fallback bool) bool {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().(bool); ok {
			return value
		}
	}
	return fallback
}

func uint32Value(values map[string]dbus.Variant, key string) uint32 {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().(uint32); ok {
			return value
		}
	}
	return 0
}

func int32Value(values map[string]dbus.Variant, key string) int32 {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().(int32); ok {
			return value
		}
	}
	return 0
}

func byteValue(values map[string]dbus.Variant, key string) uint8 {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().(byte); ok {
			return value
		}
	}
	return 0
}

func objectPathValue(values map[string]dbus.Variant, key string) dbus.ObjectPath {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().(dbus.ObjectPath); ok {
			return value
		}
	}
	return ""
}

func objectPathSliceValue(values map[string]dbus.Variant, key string) []dbus.ObjectPath {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().([]dbus.ObjectPath); ok {
			return value
		}
	}
	return nil
}

func ssidValue(values map[string]dbus.Variant, key string) string {
	if variant, ok := values[key]; ok {
		if value, ok := variant.Value().([]byte); ok {
			return strings.ToValidUTF8(string(value), "�")
		}
	}
	return ""
}

func validObjectPath(path dbus.ObjectPath) bool {
	return path != "" && path != "/"
}
