// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

type fakeSource struct {
	snapshot        model.Snapshot
	ethernetProfile model.EthernetProfile
	loadErr         error
	saveErr         error
	saved           *model.EthernetProfile
	wifiNetworks    []model.WiFiNetwork
}

func (f *fakeSource) Snapshot(context.Context) (model.Snapshot, error) {
	return f.snapshot, nil
}

func (f *fakeSource) EthernetProfile(context.Context, string, string) (model.EthernetProfile, error) {
	if f.loadErr != nil {
		return model.EthernetProfile{}, f.loadErr
	}
	return f.ethernetProfile, nil
}

func (f *fakeSource) SaveEthernetProfile(_ context.Context, profile model.EthernetProfile) (model.EthernetProfile, error) {
	if f.saveErr != nil {
		return model.EthernetProfile{}, f.saveErr
	}
	f.saved = &profile
	return profile, nil
}

func (f *fakeSource) WiFiNetworks(context.Context, string) ([]model.WiFiNetwork, error) {
	return f.wifiNetworks, nil
}

func (f *fakeSource) RequestWiFiScan(context.Context, string) error {
	return nil
}

func (f *fakeSource) SetWirelessEnabled(_ context.Context, enabled bool) error {
	f.snapshot.WirelessEnabled = enabled
	return nil
}

func (f *fakeSource) ActivateWiFiProfile(context.Context, string, string, string) error {
	return nil
}

func (f *fakeSource) DisconnectWiFi(context.Context, string) error {
	return nil
}

func (f *fakeSource) ForgetWiFiProfile(context.Context, string) error {
	return nil
}

func (f *fakeSource) UpdateWiFiProfileMetadata(context.Context, model.WiFiProfileUpdate) error {
	return nil
}

func (f *fakeSource) ConnectWiFi(context.Context, model.WiFiConnectRequest) (string, string, error) {
	return "/profile", "/active", nil
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
		Profiles: []model.ConnectionProfile{
			{
				ObjectPath:          "/org/freedesktop/NetworkManager/Settings/1",
				ID:                  "Wired connection 1",
				UUID:                "11111111-2222-3333-4444-555555555555",
				Type:                "802-3-ethernet",
				InterfaceName:       "enp2s0",
				Autoconnect:         true,
				AutoconnectPriority: 0,
			},
		},
		Devices: []model.Device{
			{
				ObjectPath:      "/org/freedesktop/NetworkManager/Devices/2",
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
					ObjectPath:    "/org/freedesktop/NetworkManager/Settings/1",
					ID:            "Wired connection 1",
					UUID:          "11111111-2222-3333-4444-555555555555",
					Type:          "802-3-ethernet",
					InterfaceName: "enp2s0",
					Autoconnect:   true,
				},
				AvailableProfileUUIDs: []string{
					"11111111-2222-3333-4444-555555555555",
				},
			},
			{
				ObjectPath: "/org/freedesktop/NetworkManager/Devices/3",
				Interface:  "wlan0",
				Kind:       model.DeviceKindWiFi,
				State:      30,
				Wireless:   &model.WirelessState{},
			},
		},
	}
}

func TestRenderShowsFriendlyNetworkSummary(t *testing.T) {
	snapshot := sampleSnapshot()
	source := &fakeSource{snapshot: snapshot}
	m := New(source)
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

	m := New(&fakeSource{snapshot: snapshot})
	m.snapshot = snapshot

	if got := len(m.visibleDevices()); got != 2 {
		t.Fatalf("visibleDevices() length = %d, want 2", got)
	}
}

func TestExpandedDetailsShowReadOnlyMetadata(t *testing.T) {
	snapshot := sampleSnapshot()
	m := New(&fakeSource{snapshot: snapshot})
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
	m := New(&fakeSource{snapshot: snapshot})
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

func TestEnterOnEthernetLoadsInteractiveForm(t *testing.T) {
	snapshot := sampleSnapshot()
	source := &fakeSource{
		snapshot:        snapshot,
		ethernetProfile: model.EthernetProfile{
			ProfilePath:   "/org/freedesktop/NetworkManager/Settings/1",
			DevicePath:    "/org/freedesktop/NetworkManager/Devices/2",
			ID:            "Wired connection 1",
			UUID:          "11111111-2222-3333-4444-555555555555",
			InterfaceName: "enp2s0",
			Autoconnect:   true,
			IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
			IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
		},
	}

	m := New(source)
	m.snapshot = snapshot
	m.loading = false

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(Model)
	if !m.formLoading || cmd == nil {
		t.Fatal("Enter on Ethernet should load its saved profile")
	}

	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.form == nil {
		t.Fatal("saved Ethernet profile should open the interactive form")
	}
	if m.form.profile.InterfaceName != "enp2s0" {
		t.Fatalf("form interface = %q", m.form.profile.InterfaceName)
	}
}

func TestEnterOnEthernetWithoutProfileStartsDHCPForm(t *testing.T) {
	snapshot := sampleSnapshot()
	snapshot.Profiles = nil
	snapshot.Devices[0].ActiveConnection = nil
	snapshot.Devices[0].AvailableProfileUUIDs = nil

	m := New(&fakeSource{snapshot: snapshot})
	m.snapshot = snapshot
	m.loading = false

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(Model)

	if cmd != nil {
		t.Fatal("new Ethernet profile should not require an existing-profile load")
	}
	if m.form == nil {
		t.Fatal("new Ethernet device should open a form")
	}
	if m.form.profile.ProfilePath != "" {
		t.Fatalf("new profile path = %q, want empty", m.form.profile.ProfilePath)
	}
	if m.form.ipv4Method != model.IPMethodAuto || m.form.ipv6Method != model.IPMethodAuto {
		t.Fatal("new Ethernet profile should default to automatic IPv4 and IPv6")
	}
}

func TestPreferredEthernetProfileUsesHighestAutoconnectPriority(t *testing.T) {
	snapshot := sampleSnapshot()
	snapshot.Devices[0].ActiveConnection = nil
	snapshot.Devices[0].AvailableProfileUUIDs = []string{"low", "high"}
	snapshot.Profiles = []model.ConnectionProfile{
		{
			ObjectPath:          "/low",
			ID:                  "Low",
			UUID:                "low",
			Type:                "802-3-ethernet",
			Autoconnect:         true,
			AutoconnectPriority: 0,
		},
		{
			ObjectPath:          "/high",
			ID:                  "High",
			UUID:                "high",
			Type:                "802-3-ethernet",
			Autoconnect:         true,
			AutoconnectPriority: 10,
		},
	}

	m := New(&fakeSource{snapshot: snapshot})
	m.snapshot = snapshot

	profile := m.preferredEthernetProfile(snapshot.Devices[0])
	if profile == nil || profile.UUID != "high" {
		t.Fatalf("preferred profile = %#v, want high priority profile", profile)
	}
}

func TestEnterOnWiFiOpensManager(t *testing.T) {
	snapshot := sampleSnapshot()
	source := &fakeSource{
		snapshot:     snapshot,
		wifiNetworks: []model.WiFiNetwork{
			{
				ObjectPath:    "/org/freedesktop/NetworkManager/AccessPoint/1",
				SSID:          "Home Wi-Fi",
				Strength:      82,
				Security:      model.WiFiSecurityPersonal,
				KeyManagement: "wpa-psk",
			},
		},
	}

	m := New(source)
	m.snapshot = snapshot
	m.loading = false
	m.cursor = 1

	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = updated.(Model)
	if m.wifi == nil {
		t.Fatal("Enter on Wi-Fi should open the Wi-Fi manager")
	}
	if cmd == nil {
		t.Fatal("Wi-Fi manager should refresh networks when opened")
	}
}
