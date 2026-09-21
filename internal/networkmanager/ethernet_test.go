// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"reflect"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestParseEthernetProfileUsesSelectedDeviceInterface(t *testing.T) {
	settings := ethernetSettingsFixture()
	delete(settings["connection"], "interface-name")

	profile, err := parseEthernetProfile(
		settings,
		"/org/freedesktop/NetworkManager/Settings/1",
		"/org/freedesktop/NetworkManager/Devices/2",
		"enp2s0",
	)
	if err != nil {
		t.Fatalf("parseEthernetProfile() error = %v", err)
	}

	if profile.InterfaceName != "enp2s0" {
		t.Fatalf("InterfaceName = %q, want enp2s0", profile.InterfaceName)
	}
	if profile.IPv4.Method != model.IPMethodManual {
		t.Fatalf("IPv4 method = %q, want manual", profile.IPv4.Method)
	}
	if len(profile.IPv4.Addresses) != 1 || profile.IPv4.Addresses[0].Address != "192.168.0.59" {
		t.Fatalf("IPv4 addresses = %#v", profile.IPv4.Addresses)
	}
}

func TestParseEthernetProfileRejects8021X(t *testing.T) {
	settings := ethernetSettingsFixture()
	settings["802-1x"] = map[string]dbus.Variant{
		"identity": dbus.MakeVariant("user"),
	}

	if _, err := parseEthernetProfile(settings, "/profile", "/device", "enp2s0"); err == nil {
		t.Fatal("expected 802.1X Ethernet profile to be rejected")
	}
}

func TestPatchEthernetSettingsPreservesUnrelatedSettings(t *testing.T) {
	settings := ethernetSettingsFixture()
	settings["proxy"] = map[string]dbus.Variant{
		"method": dbus.MakeVariant(int32(0)),
	}
	settings["ipv4"]["addresses"] = dbus.MakeVariant([]uint32{1, 2, 3})
	settings["ipv4"]["dns"] = dbus.MakeVariant([]uint32{0x01010101})

	profile := model.EthernetProfile{
		ProfilePath:   "/org/freedesktop/NetworkManager/Settings/1",
		DevicePath:    "/org/freedesktop/NetworkManager/Devices/2",
		ID:            "Wired connection 1",
		UUID:          "11111111-2222-3333-4444-555555555555",
		InterfaceName: "enp2s0",
		Autoconnect:   false,
		MTU:           9000,
		IPv4: model.IPProfileConfig{
			Method: model.IPMethodAuto,
			DNS:    []string{"1.1.1.1", "9.9.9.9"},
		},
		IPv6: model.IPProfileConfig{
			Method: model.IPMethodDisabled,
		},
	}

	patchEthernetSettings(settings, profile)

	if _, ok := settings["proxy"]; !ok {
		t.Fatal("unrelated proxy setting was removed")
	}
	if got := stringValue(settings["connection"], "id"); got != "Wired connection 1" {
		t.Fatalf("connection id changed to %q", got)
	}
	if got := uint32Value(settings["802-3-ethernet"], "mtu"); got != 9000 {
		t.Fatalf("MTU = %d, want 9000", got)
	}
	if got := boolValue(settings["connection"], "autoconnect"); got {
		t.Fatal("autoconnect should be false")
	}
	if _, ok := settings["ipv4"]["addresses"]; ok {
		t.Fatal("legacy IPv4 addresses key should be removed")
	}
	if _, ok := settings["ipv4"]["dns"]; ok {
		t.Fatal("legacy IPv4 dns key should be removed")
	}
	if _, ok := settings["ipv4"]["address-data"]; ok {
		t.Fatal("automatic IPv4 must not keep manual address-data")
	}
	if got := stringSliceValue(settings["ipv4"], "dns-data"); !reflect.DeepEqual(got, []string{"1.1.1.1", "9.9.9.9"}) {
		t.Fatalf("IPv4 dns-data = %#v", got)
	}
	if !boolValue(settings["ipv4"], "ignore-auto-dns") {
		t.Fatal("custom automatic IPv4 DNS should ignore DHCP DNS")
	}
	if _, ok := settings["ipv6"]["route-data"]; ok {
		t.Fatal("disabled IPv6 must not keep route-data")
	}
}

func TestPatchEthernetSettingsWritesManualAddressData(t *testing.T) {
	settings := ethernetSettingsFixture()
	profile := model.EthernetProfile{
		Autoconnect: true,
		IPv4: model.IPProfileConfig{
			Method: model.IPMethodManual,
			Addresses: []model.IPAddress{
				{Address: "10.20.30.40", Prefix: 24},
			},
			Gateway: "10.20.30.1",
			DNS:     []string{"10.20.30.1"},
		},
		IPv6: model.IPProfileConfig{Method: model.IPMethodAuto},
	}

	patchEthernetSettings(settings, profile)

	addressData, ok := settings["ipv4"]["address-data"].Value().([]map[string]dbus.Variant)
	if !ok || len(addressData) != 1 {
		t.Fatalf("address-data = %#v", settings["ipv4"]["address-data"].Value())
	}
	if got := stringValue(addressData[0], "address"); got != "10.20.30.40" {
		t.Fatalf("manual address = %q", got)
	}
	if got := stringValue(settings["ipv4"], "gateway"); got != "10.20.30.1" {
		t.Fatalf("gateway = %q", got)
	}
}

func ethernetSettingsFixture() map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{
		"connection": {
			"id":             dbus.MakeVariant("Wired connection 1"),
			"uuid":           dbus.MakeVariant("11111111-2222-3333-4444-555555555555"),
			"type":           dbus.MakeVariant("802-3-ethernet"),
			"interface-name": dbus.MakeVariant("enp2s0"),
			"autoconnect":    dbus.MakeVariant(true),
		},
		"802-3-ethernet": {
			"mtu": dbus.MakeVariant(uint32(1500)),
		},
		"ipv4": {
			"method": dbus.MakeVariant("manual"),
			"address-data": dbus.MakeVariant([]map[string]dbus.Variant{
				{
					"address": dbus.MakeVariant("192.168.0.59"),
					"prefix":  dbus.MakeVariant(uint32(24)),
				},
			}),
			"gateway":  dbus.MakeVariant("192.168.0.1"),
			"dns-data": dbus.MakeVariant([]string{"192.168.0.1"}),
		},
		"ipv6": {
			"method": dbus.MakeVariant("auto"),
			"route-data": dbus.MakeVariant([]map[string]dbus.Variant{
				{"dest": dbus.MakeVariant("2001:db8::/64")},
			}),
		},
	}
}

func TestNewEthernetSettingsCreatesPlainWiredProfile(t *testing.T) {
	profile := model.EthernetProfile{
		ID:            "Ethernet enp2s0",
		UUID:          "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		InterfaceName: "enp2s0",
		Autoconnect:   true,
		MTU:           1500,
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	}

	settings := newEthernetSettings(profile)
	if got := stringValue(settings["connection"], "type"); got != "802-3-ethernet" {
		t.Fatalf("connection type = %q", got)
	}
	if got := stringValue(settings["connection"], "interface-name"); got != "enp2s0" {
		t.Fatalf("interface-name = %q", got)
	}
	if got := stringValue(settings["connection"], "uuid"); got != profile.UUID {
		t.Fatalf("uuid = %q", got)
	}
	if got := stringValue(settings["ipv4"], "method"); got != "auto" {
		t.Fatalf("IPv4 method = %q", got)
	}
}

func TestNewUUIDIsVersion4Shape(t *testing.T) {
	uuid, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("newUUID() = %q", uuid)
	}
	if uuid[14] != '4' {
		t.Fatalf("newUUID() version nibble = %q", uuid[14])
	}
	if !strings.Contains("89ab", strings.ToLower(string(uuid[19]))) {
		t.Fatalf("newUUID() variant nibble = %q", uuid[19])
	}
}
