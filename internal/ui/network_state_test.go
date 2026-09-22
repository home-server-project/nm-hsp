// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"testing"
	"time"

	"github.com/home-server-project/nm-hsp/internal/model"
)

type sequenceSnapshotSource struct {
	snapshots []model.Snapshot
	index     int
}

func (s *sequenceSnapshotSource) Snapshot(context.Context) (model.Snapshot, error) {
	if len(s.snapshots) == 0 {
		return model.Snapshot{}, nil
	}
	index := s.index
	if index >= len(s.snapshots) {
		index = len(s.snapshots) - 1
	}
	if s.index+1 < len(s.snapshots) {
		s.index++
	}
	return s.snapshots[index], nil
}

func stateSnapshot(state uint32, active *model.ConnectionProfile, ssid string) model.Snapshot {
	device := model.Device{
		ObjectPath:       "/device",
		Interface:        "wlan0",
		Kind:             model.DeviceKindWiFi,
		State:            state,
		ActiveConnection: active,
		Wireless:         &model.WirelessState{SSID: ssid},
	}
	return model.Snapshot{Devices: []model.Device{device}}
}

func TestWaitForDeviceActivationRequiresRealActivatedState(t *testing.T) {
	profile := &model.ConnectionProfile{UUID: "wifi-uuid"}
	source := &sequenceSnapshotSource{snapshots: []model.Snapshot{
		stateSnapshot(30, nil, ""),
		stateSnapshot(50, profile, ""),
		stateSnapshot(70, profile, ""),
		stateSnapshot(100, profile, "Home"),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, device, err := waitForDeviceActivation(ctx, source, "/device", "wlan0", "wifi-uuid", "Home")
	if err != nil {
		t.Fatalf("waitForDeviceActivation() error = %v", err)
	}
	if device.State != 100 || device.Wireless == nil || device.Wireless.SSID != "Home" {
		t.Fatalf("activation settled on %#v", device)
	}
}

func TestWaitForDeviceActivationReportsFailedTransition(t *testing.T) {
	profile := &model.ConnectionProfile{UUID: "wifi-uuid"}
	source := &sequenceSnapshotSource{snapshots: []model.Snapshot{
		stateSnapshot(50, profile, ""),
		stateSnapshot(120, nil, ""),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, _, err := waitForDeviceActivation(ctx, source, "/device", "wlan0", "wifi-uuid", "")
	if err == nil {
		t.Fatal("failed NetworkManager activation must not be reported as connected")
	}
}

func TestWaitForDeviceDisconnectedTreatsAlreadyDisconnectedAsSuccess(t *testing.T) {
	source := &sequenceSnapshotSource{snapshots: []model.Snapshot{
		stateSnapshot(30, nil, ""),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, device, err := waitForDeviceDisconnected(ctx, source, "/device", "wlan0")
	if err != nil {
		t.Fatalf("waitForDeviceDisconnected() error = %v", err)
	}
	if device.State != 30 {
		t.Fatalf("disconnect settled on state %d", device.State)
	}
}
