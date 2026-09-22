// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (r systemdReader) EnableAndStart(ctx context.Context, unit string) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("connect system bus: %w", err)
	}
	defer conn.Close()

	manager := conn.Object(systemdBusName, systemdManagerPath)
	if call := manager.CallWithContext(ctx, systemdManagerIface+".EnableUnitFiles", 0, []string{unit}, false, true); call.Err != nil {
		return fmt.Errorf("enable unit: %w", call.Err)
	}
	if call := manager.CallWithContext(ctx, systemdManagerIface+".StartUnit", 0, unit, "replace"); call.Err != nil {
		return fmt.Errorf("start unit: %w", call.Err)
	}
	return r.waitForActiveState(ctx, unit, true)
}

func (r systemdReader) StopAndDisable(ctx context.Context, unit string) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("connect system bus: %w", err)
	}
	defer conn.Close()

	manager := conn.Object(systemdBusName, systemdManagerPath)
	if call := manager.CallWithContext(ctx, systemdManagerIface+".StopUnit", 0, unit, "replace"); call.Err != nil {
		return fmt.Errorf("stop unit: %w", call.Err)
	}
	if err := r.waitForActiveState(ctx, unit, false); err != nil {
		return err
	}
	if call := manager.CallWithContext(ctx, systemdManagerIface+".DisableUnitFiles", 0, []string{unit}, false); call.Err != nil {
		return fmt.Errorf("disable unit: %w", call.Err)
	}
	return nil
}

func (r systemdReader) waitForActiveState(ctx context.Context, unit string, wantActive bool) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		state, err := r.UnitState(ctx, unit)
		if err == nil && (state.active == "active") == wantActive {
			return nil
		}

		select {
		case <-ctx.Done():
			if wantActive {
				return fmt.Errorf("service did not become active: %w", ctx.Err())
			}
			return fmt.Errorf("service did not stop: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func isNoSuchSystemdUnit(err error) bool {
	var dbusErr *dbus.Error
	return errors.As(err, &dbusErr) && dbusErr.Name == "org.freedesktop.systemd1.NoSuchUnit"
}
