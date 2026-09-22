// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	systemdBusName      = "org.freedesktop.systemd1"
	systemdManagerPath  = dbus.ObjectPath("/org/freedesktop/systemd1")
	systemdManagerIface = "org.freedesktop.systemd1.Manager"
	systemdUnitIface    = "org.freedesktop.systemd1.Unit"
)

type systemdReader struct{}

func (systemdReader) UnitState(ctx context.Context, unit string) (serviceState, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return serviceState{}, fmt.Errorf("connect system bus: %w", err)
	}
	defer conn.Close()

	manager := conn.Object(systemdBusName, systemdManagerPath)
	var unitFileState string
	if err := manager.CallWithContext(ctx, systemdManagerIface+".GetUnitFileState", 0, unit).Store(&unitFileState); err != nil {
		return serviceState{}, fmt.Errorf("read unit file state: %w", err)
	}

	var unitPath dbus.ObjectPath
	if err := manager.CallWithContext(ctx, systemdManagerIface+".GetUnit", 0, unit).Store(&unitPath); err != nil {
		if isNoSuchSystemdUnit(err) {
			return serviceState{enabled: unitFileState == "enabled", active: "inactive"}, nil
		}
		return serviceState{}, fmt.Errorf("resolve unit: %w", err)
	}

	var activeProperty dbus.Variant
	if err := conn.Object(systemdBusName, unitPath).CallWithContext(
		ctx,
		"org.freedesktop.DBus.Properties.Get",
		0,
		systemdUnitIface,
		"ActiveState",
	).Store(&activeProperty); err != nil {
		return serviceState{}, fmt.Errorf("read active state: %w", err)
	}
	active, ok := activeProperty.Value().(string)
	if !ok {
		return serviceState{}, fmt.Errorf("active state has unexpected type")
	}

	return serviceState{
		enabled: unitFileState == "enabled",
		active:  active,
	}, nil
}

func isNoSuchSystemdUnit(err error) bool {
	var dbusErr *dbus.Error
	return errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit"
}
