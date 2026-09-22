// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
)

// ApplyRepair executes one explicitly approved diagnostics repair. Every repair
// re-validates the current NetworkManager state before modifying anything.
func (c *Client) ApplyRepair(ctx context.Context, action model.RepairAction) error {
	switch action.Kind {
	case model.RepairCreateDHCPProfile:
		return c.repairCreateDHCPProfile(ctx, action)
	case model.RepairEnableAutoconnect:
		return c.repairEnableAutoconnect(ctx, action)
	case model.RepairActivateProfile:
		return c.repairActivateProfile(ctx, action)
	case model.RepairFixInterfaceBinding:
		return c.repairFixInterfaceBinding(ctx, action)
	case model.RepairEnableWiFi:
		return c.SetWirelessEnabled(ctx, true)
	default:
		return fmt.Errorf("unsupported diagnostics repair %q", action.Kind)
	}
}

func (c *Client) repairCreateDHCPProfile(ctx context.Context, action model.RepairAction) error {
	devicePath, props, err := c.repairEthernetDevice(ctx, action)
	if err != nil {
		return err
	}
	if path := objectPathValue(props, "ActiveConnection"); validObjectPath(path) {
		return fmt.Errorf("Ethernet state changed: %s already has an active connection; run diagnostics again", action.InterfaceName)
	}

	for _, profilePath := range objectPathSliceValue(props, "AvailableConnections") {
		settings, err := c.connectionSettings(ctx, profilePath)
		if err != nil {
			return err
		}
		if stringValue(settings["connection"], "type") == "802-3-ethernet" {
			return fmt.Errorf("Ethernet state changed: a compatible saved profile now exists; run diagnostics again")
		}
	}

	wired, err := c.getAll(ctx, devicePath, wiredInterface)
	if err != nil {
		return fmt.Errorf("read Ethernet link before repair: %w", err)
	}
	if !boolValue(wired, "Carrier") {
		return fmt.Errorf("Ethernet cable is not connected; profile creation repair was not applied")
	}

	profile := model.EthernetProfile{
		DevicePath:    action.DevicePath,
		InterfaceName: action.InterfaceName,
		Autoconnect:   true,
		IPv4: model.IPProfileConfig{
			Method: model.IPMethodAuto,
		},
		IPv6: model.IPProfileConfig{
			Method: model.IPMethodAuto,
		},
	}
	created, err := c.CreateEthernetProfile(ctx, profile)
	if err != nil {
		return err
	}
	if err := c.activateSavedProfile(ctx, created.ProfilePath, action.DevicePath); err != nil {
		return fmt.Errorf("created Ethernet profile %q but could not activate it: %w", created.ID, err)
	}
	return nil
}

func (c *Client) repairEnableAutoconnect(ctx context.Context, action model.RepairAction) error {
	path, settings, err := c.repairEthernetProfile(ctx, action)
	if err != nil {
		return err
	}
	connection := settings["connection"]
	if boolValueDefault(connection, "autoconnect", true) {
		return nil
	}
	connection["autoconnect"] = dbus.MakeVariant(true)
	return c.persistRepairSettings(ctx, path, settings, "enable autoconnect")
}

func (c *Client) repairActivateProfile(ctx context.Context, action model.RepairAction) error {
	_, _, err := c.repairEthernetProfile(ctx, action)
	if err != nil {
		return err
	}
	_, _, err = c.repairEthernetDevice(ctx, action)
	if err != nil {
		return err
	}
	return c.activateSavedProfile(ctx, action.ProfilePath, action.DevicePath)
}

func (c *Client) repairFixInterfaceBinding(ctx context.Context, action model.RepairAction) error {
	path, settings, err := c.repairEthernetProfile(ctx, action)
	if err != nil {
		return err
	}
	_, deviceProps, err := c.repairEthernetDevice(ctx, action)
	if err != nil {
		return err
	}

	connection := settings["connection"]
	currentInterface := stringValue(connection, "interface-name")
	if currentInterface == action.InterfaceName {
		return nil
	}
	if action.Before != "" && currentInterface != action.Before {
		return fmt.Errorf(
			"Ethernet state changed: profile is now bound to %q instead of %q; run diagnostics again",
			currentInterface,
			action.Before,
		)
	}

	profileMAC := hardwareAddressValue(settings["802-3-ethernet"], "mac-address")
	deviceMAC := stringValue(deviceProps, "HwAddress")
	if profileMAC == "" {
		return fmt.Errorf("refusing interface repair: saved profile has no hardware-address binding to verify")
	}
	if !sameHardwareAddress(profileMAC, deviceMAC) {
		return fmt.Errorf(
			"refusing interface repair: profile hardware address %s does not match adapter %s",
			profileMAC,
			deviceMAC,
		)
	}

	connection["interface-name"] = dbus.MakeVariant(action.InterfaceName)
	return c.persistRepairSettings(ctx, path, settings, "update Ethernet interface binding")
}

func (c *Client) repairEthernetProfile(
	ctx context.Context,
	action model.RepairAction,
) (dbus.ObjectPath, map[string]map[string]dbus.Variant, error) {
	path := dbus.ObjectPath(action.ProfilePath)
	if !path.IsValid() || path == "/" {
		return "", nil, fmt.Errorf("invalid Ethernet profile path %q", action.ProfilePath)
	}
	settings, err := c.connectionSettings(ctx, path)
	if err != nil {
		return "", nil, err
	}
	connection := settings["connection"]
	if stringValue(connection, "type") != "802-3-ethernet" {
		return "", nil, fmt.Errorf("diagnostics repair target is no longer an Ethernet profile")
	}
	if action.ProfileID != "" && stringValue(connection, "id") != action.ProfileID {
		return "", nil, fmt.Errorf("Ethernet profile changed while diagnostics were open; run diagnostics again")
	}
	return path, settings, nil
}

func (c *Client) repairEthernetDevice(
	ctx context.Context,
	action model.RepairAction,
) (dbus.ObjectPath, map[string]dbus.Variant, error) {
	path := dbus.ObjectPath(action.DevicePath)
	if !path.IsValid() || path == "/" {
		return "", nil, fmt.Errorf("invalid Ethernet device path %q", action.DevicePath)
	}
	props, err := c.getAll(ctx, path, deviceInterface)
	if err != nil {
		return "", nil, fmt.Errorf("read Ethernet device before repair: %w", err)
	}
	if uint32Value(props, "DeviceType") != 1 {
		return "", nil, fmt.Errorf("diagnostics repair target is no longer an Ethernet device")
	}
	if !boolValue(props, "Managed") {
		return "", nil, fmt.Errorf("NetworkManager is no longer managing this Ethernet adapter")
	}
	if action.InterfaceName != "" && stringValue(props, "Interface") != action.InterfaceName {
		return "", nil, fmt.Errorf(
			"Ethernet device changed from %q to %q; run diagnostics again",
			action.InterfaceName,
			stringValue(props, "Interface"),
		)
	}
	return path, props, nil
}

func (c *Client) activateSavedProfile(ctx context.Context, profilePath, devicePath string) error {
	profile := dbus.ObjectPath(profilePath)
	device := dbus.ObjectPath(devicePath)
	if !profile.IsValid() || profile == "/" {
		return fmt.Errorf("invalid connection profile path %q", profilePath)
	}
	if !device.IsValid() || device == "/" {
		return fmt.Errorf("invalid device path %q", devicePath)
	}

	var active dbus.ObjectPath
	if err := c.call(
		ctx,
		managerPath,
		managerInterface+".ActivateConnection",
		profile,
		device,
		dbus.ObjectPath("/"),
	).Store(&active); err != nil {
		return fmt.Errorf("activate Ethernet profile: %w", err)
	}
	return nil
}

func (c *Client) persistRepairSettings(
	ctx context.Context,
	path dbus.ObjectPath,
	settings map[string]map[string]dbus.Variant,
	action string,
) error {
	var result map[string]dbus.Variant
	if err := c.call(
		ctx,
		path,
		connectionInterface+".Update2",
		settings,
		update2ToDisk,
		map[string]dbus.Variant{},
	).Store(&result); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}

func sameHardwareAddress(left, right string) bool {
	normalize := func(value string) string {
		return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", ":"))
	}
	return left != "" && right != "" && normalize(left) == normalize(right)
}
