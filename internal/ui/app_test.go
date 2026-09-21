// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

type fakeSource struct {
	snapshot model.Snapshot
}

func (f fakeSource) Snapshot(context.Context) (model.Snapshot, error) {
	return f.snapshot, nil
}

func boolPtr(value bool) *bool {
	return &value
}

func sampleSnapshot() model.Snapshot {
	return model.Snapshot{
		Version:           "1.56.0",
		Connectivity:      4,
		NetworkingEnabled: true,
		WirelessEnabled:   true,
		Devices: []model.Device{
			{
				Interface:       "enp2s0",
				Kind:            model.DeviceKindEthernet,
				State:           100,
				Carrier:         boolPtr(true),
				SpeedMbps:       1000,
				HardwareAddress: "90:8D:6E:8A:C0:B1",
				MTU:             1500,
				IPv4: model.IPConfig{
					Addresses: []model.IPAddress{
						{Address: "192.168.0.59", Prefix: 24},
					},
					Gateway: "192.168.0.1",
					DNS:     []string{"192.168.0.1"},
				},
				ActiveConnection: &model.ConnectionProfile{
					ID:          "Wired connection 1",
					Autoconnect: true,
				},
			},
			{
				Interface: "wlan0",
				Kind:      model.DeviceKindWiFi,
				State:     30,
				Wireless:  &model.WirelessState{},
			},
		},
	}
}

func TestRenderShowsFriendlyNetworkSummary(t *testing.T) {
	snapshot := sampleSnapshot()
	m := New(fakeSource{snapshot: snapshot})
	m.snapshot = snapshot
	m.loading = false
	m.width = 100

	output := m.render()
	for _, want := range []string{
		"NetworkManager-HSP",
		"Internet connected",
		"Ethernet",
		"enp2s0",
		"192.168.0.59/24",
		"Wi-Fi",
		"wlan0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("render missing %q", want)
		}
	}
}

func TestVisibleDevicesFiltersNonEthernetAndNonWiFi(t *testing.T) {
	snapshot := sampleSnapshot()
	snapshot.Devices = append(snapshot.Devices, model.Device{
		Interface: "lo",
		Kind:      model.DeviceKindOther,
	})

	m := New(fakeSource{snapshot: snapshot})
	m.snapshot = snapshot

	if got := len(m.visibleDevices()); got != 2 {
		t.Fatalf("visibleDevices() length = %d, want 2", got)
	}
}

func TestExpandedDetailsShowReadOnlyMetadata(t *testing.T) {
	snapshot := sampleSnapshot()
	m := New(fakeSource{snapshot: snapshot})
	m.snapshot = snapshot
	m.loading = false
	m.width = 100
	m.expanded = true

	output := m.render()
	for _, want := range []string{
		"90:8D:6E:8A:C0:B1",
		"Gateway",
		"192.168.0.1",
		"DNS",
		"Autoconnect",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expanded render missing %q", want)
		}
	}
}

func TestCompactRenderKeepsEssentialFields(t *testing.T) {
	snapshot := sampleSnapshot()
	m := New(fakeSource{snapshot: snapshot})
	m.snapshot = snapshot
	m.loading = false
	m.width = 48

	output := m.render()
	for _, want := range []string{
		"enp2s0",
		"connected",
		"192.168.0.59/24",
		"q exit",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("compact render missing %q", want)
		}
	}
}
