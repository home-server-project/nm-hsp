// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/home-server-project/nm-hsp/internal/model"
)

type networkSnapshotSource interface {
	Snapshot(context.Context) (model.Snapshot, error)
}

func currentDeviceSnapshot(
	ctx context.Context,
	source networkSnapshotSource,
	devicePath, interfaceName string,
) (model.Snapshot, model.Device, error) {
	snapshot, err := source.Snapshot(ctx)
	if err != nil {
		return model.Snapshot{}, model.Device{}, err
	}
	device, ok := findDevice(snapshot.Devices, devicePath, interfaceName)
	if !ok {
		return snapshot, model.Device{}, fmt.Errorf("network adapter %s is no longer available", interfaceName)
	}
	return snapshot, device, nil
}

func waitForDeviceActivation(
	ctx context.Context,
	source networkSnapshotSource,
	devicePath, interfaceName, profileUUID, ssid string,
) (model.Snapshot, model.Device, error) {
	var lastSnapshot model.Snapshot
	var lastDevice model.Device
	seenTransition := false

	for {
		snapshot, device, err := currentDeviceSnapshot(ctx, source, devicePath, interfaceName)
		if err != nil {
			return lastSnapshot, lastDevice, err
		}
		lastSnapshot = snapshot
		lastDevice = device

		if device.ActiveConnection != nil || (device.State >= 40 && device.State <= 110) {
			seenTransition = true
		}
		if activationMatches(device, profileUUID, ssid) {
			return snapshot, device, nil
		}
		switch device.State {
		case 10, 20, 120:
			return snapshot, device, fmt.Errorf(
				"connection failed: %s is %s",
				emptyFallback(interfaceName, "adapter"),
				model.DeviceStateName(device.State),
			)
		case 30:
			if seenTransition {
				return snapshot, device, fmt.Errorf(
					"connection failed: %s returned to disconnected state",
					emptyFallback(interfaceName, "adapter"),
				)
			}
		}

		select {
		case <-ctx.Done():
			return lastSnapshot, lastDevice, fmt.Errorf(
				"connection did not become active: last state %s",
				model.DeviceStateName(lastDevice.State),
			)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func waitForDeviceDisconnected(
	ctx context.Context,
	source networkSnapshotSource,
	devicePath, interfaceName string,
) (model.Snapshot, model.Device, error) {
	var lastSnapshot model.Snapshot
	var lastDevice model.Device

	for {
		snapshot, device, err := currentDeviceSnapshot(ctx, source, devicePath, interfaceName)
		if err != nil {
			return lastSnapshot, lastDevice, err
		}
		lastSnapshot = snapshot
		lastDevice = device

		if deviceDisconnected(device) {
			return snapshot, device, nil
		}

		select {
		case <-ctx.Done():
			return lastSnapshot, lastDevice, fmt.Errorf(
				"disconnect did not complete: last state %s",
				model.DeviceStateName(lastDevice.State),
			)
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func activationMatches(device model.Device, profileUUID, ssid string) bool {
	if device.State != 100 || device.ActiveConnection == nil {
		return false
	}
	if profileUUID != "" && device.ActiveConnection.UUID != profileUUID {
		return false
	}
	if ssid != "" {
		if device.Wireless == nil || device.Wireless.SSID != ssid {
			return false
		}
	}
	return true
}

func deviceDisconnected(device model.Device) bool {
	if device.State == 10 || device.State == 20 || device.State == 30 {
		return true
	}
	return device.ActiveConnection == nil && device.State < 40
}

func operationStateError(action string, callErr, stateErr error) error {
	if stateErr == nil {
		return nil
	}
	if callErr != nil {
		return fmt.Errorf("%s: %w", action, callErr)
	}
	return stateErr
}
