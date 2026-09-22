// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/security"
	"github.com/home-server-project/nm-hsp/internal/validation"
)

// SetWirelessEnabled turns the global NetworkManager Wi-Fi radio state on or off.
func (c *Client) SetWirelessEnabled(ctx context.Context, enabled bool) error {
	if err := c.call(
		ctx,
		managerPath,
		propertiesInterface+".Set",
		managerInterface,
		"WirelessEnabled",
		dbus.MakeVariant(enabled),
	).Err; err != nil {
		return fmt.Errorf("set Wi-Fi radio state: %w", err)
	}
	return nil
}

// ActivateWiFiProfile activates a saved Wi-Fi profile on the selected device.
func (c *Client) ActivateWiFiProfile(ctx context.Context, profilePath, devicePath, accessPointPath string) error {
	profile := dbus.ObjectPath(profilePath)
	device := dbus.ObjectPath(devicePath)
	if !profile.IsValid() || profile == "/" {
		return fmt.Errorf("invalid Wi-Fi profile path %q", profilePath)
	}
	if !device.IsValid() || device == "/" {
		return fmt.Errorf("invalid Wi-Fi device path %q", devicePath)
	}

	specific := dbus.ObjectPath("/")
	if accessPointPath != "" {
		candidate := dbus.ObjectPath(accessPointPath)
		if !candidate.IsValid() {
			return fmt.Errorf("invalid Wi-Fi access point path %q", accessPointPath)
		}
		specific = candidate
	}

	var active dbus.ObjectPath
	if err := c.call(
		ctx,
		managerPath,
		managerInterface+".ActivateConnection",
		profile,
		device,
		specific,
	).Store(&active); err != nil {
		return fmt.Errorf("activate Wi-Fi profile: %w", err)
	}
	return nil
}

// DisconnectWiFi deactivates the selected active connection.
func (c *Client) DisconnectWiFi(ctx context.Context, activeConnectionPath string) error {
	active := dbus.ObjectPath(activeConnectionPath)
	if !active.IsValid() || active == "/" {
		return fmt.Errorf("invalid active connection path %q", activeConnectionPath)
	}
	if err := c.call(
		ctx,
		managerPath,
		managerInterface+".DeactivateConnection",
		active,
	).Err; err != nil {
		return fmt.Errorf("disconnect Wi-Fi: %w", err)
	}
	return nil
}

// ForgetWiFiProfile deletes a saved NetworkManager Wi-Fi profile.
func (c *Client) ForgetWiFiProfile(ctx context.Context, profilePath string) error {
	path := dbus.ObjectPath(profilePath)
	if !path.IsValid() || path == "/" {
		return fmt.Errorf("invalid Wi-Fi profile path %q", profilePath)
	}
	if err := c.call(ctx, path, connectionInterface+".Delete").Err; err != nil {
		return fmt.Errorf("forget Wi-Fi profile: %w", err)
	}
	return nil
}

// UpdateWiFiProfileMetadata updates only autoconnect metadata on an existing
// Wi-Fi profile while preserving every other setting.
func (c *Client) UpdateWiFiProfileMetadata(ctx context.Context, update model.WiFiProfileUpdate) error {
	path := dbus.ObjectPath(update.ProfilePath)
	if !path.IsValid() || path == "/" {
		return fmt.Errorf("invalid Wi-Fi profile path %q", update.ProfilePath)
	}

	settings, err := c.connectionSettings(ctx, path)
	if err != nil {
		return err
	}
	if err := patchWiFiProfileMetadata(settings, update); err != nil {
		return err
	}

	var result map[string]dbus.Variant
	if err := c.call(
		ctx,
		path,
		connectionInterface+".Update2",
		settings,
		update2ToDisk,
		map[string]dbus.Variant{},
	).Store(&result); err != nil {
		return fmt.Errorf("save Wi-Fi profile metadata: %w", err)
	}
	return nil
}

func patchWiFiProfileMetadata(
	settings map[string]map[string]dbus.Variant,
	update model.WiFiProfileUpdate,
) error {
	connection := settings["connection"]
	if stringValue(connection, "type") != "802-11-wireless" {
		return fmt.Errorf("profile is not a Wi-Fi connection")
	}
	connection["autoconnect"] = dbus.MakeVariant(update.Autoconnect)
	connection["autoconnect-priority"] = dbus.MakeVariant(update.AutoconnectPriority)
	return nil
}

// ConnectWiFi creates a persistent supported Wi-Fi profile and activates it.
func (c *Client) ConnectWiFi(ctx context.Context, request model.WiFiConnectRequest) (string, string, error) {
	defer request.Password.Clear()

	if err := validation.ValidateWiFiConnectRequest(request); err != nil {
		return "", "", err
	}

	device := dbus.ObjectPath(request.DevicePath)
	if !device.IsValid() || device == "/" {
		return "", "", fmt.Errorf("invalid Wi-Fi device path %q", request.DevicePath)
	}

	specific := dbus.ObjectPath("/")
	if !request.Hidden && request.AccessPointPath != "" {
		candidate := dbus.ObjectPath(request.AccessPointPath)
		if !candidate.IsValid() {
			return "", "", fmt.Errorf("invalid Wi-Fi access point path %q", request.AccessPointPath)
		}
		specific = candidate
	}

	uuid, err := newUUID()
	if err != nil {
		return "", "", fmt.Errorf("generate Wi-Fi connection UUID: %w", err)
	}

	settings := newWiFiSettings(request, uuid)
	options := map[string]dbus.Variant{
		"persist": dbus.MakeVariant("disk"),
	}
	var profilePath dbus.ObjectPath
	var activePath dbus.ObjectPath
	var result map[string]dbus.Variant
	callErr := c.call(
		ctx,
		managerPath,
		managerInterface+".AddAndActivateConnection2",
		settings,
		device,
		specific,
		options,
	).Store(&profilePath, &activePath, &result)
	scrubWiFiSettingsSecret(settings)
	if callErr != nil {
		return "", "", sanitizeWiFiConnectError(callErr, request.Password)
	}

	return string(profilePath), string(activePath), nil
}

func scrubWiFiSettingsSecret(settings map[string]map[string]dbus.Variant) {
	wirelessSecurity := settings["802-11-wireless-security"]
	if wirelessSecurity == nil {
		return
	}
	delete(wirelessSecurity, "psk")
}

func sanitizeWiFiConnectError(err error, password security.Secret) error {
	if err == nil {
		return nil
	}
	return security.RedactError(fmt.Errorf("connect to Wi-Fi: %w", err), password)
}

func newWiFiSettings(request model.WiFiConnectRequest, uuid string) map[string]map[string]dbus.Variant {
	settings := map[string]map[string]dbus.Variant{
		"connection": {
			"id":                   dbus.MakeVariant(request.SSID),
			"uuid":                 dbus.MakeVariant(uuid),
			"type":                 dbus.MakeVariant("802-11-wireless"),
			"autoconnect":          dbus.MakeVariant(request.Autoconnect),
			"autoconnect-priority": dbus.MakeVariant(request.AutoconnectPriority),
		},
		"802-11-wireless": {
			"ssid":   dbus.MakeVariant([]byte(request.SSID)),
			"mode":   dbus.MakeVariant("infrastructure"),
			"hidden": dbus.MakeVariant(request.Hidden),
		},
		"ipv4": {
			"method": dbus.MakeVariant("auto"),
		},
		"ipv6": {
			"method": dbus.MakeVariant("auto"),
		},
	}

	if request.KeyManagement != "" {
		wirelessSecurity := map[string]dbus.Variant{
			"key-mgmt": dbus.MakeVariant(request.KeyManagement),
		}
		if request.KeyManagement == "wpa-psk" || request.KeyManagement == "sae" {
			wirelessSecurity["psk"] = dbus.MakeVariant(request.Password.Value())
		}
		settings["802-11-wireless-security"] = wirelessSecurity
	}

	return settings
}
