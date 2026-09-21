// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestParseIPConfigProperties(t *testing.T) {
	props := map[string]dbus.Variant{
		"AddressData": dbus.MakeVariant([]map[string]dbus.Variant{
			{
				"address": dbus.MakeVariant("192.168.0.50"),
				"prefix":  dbus.MakeVariant(uint32(24)),
			},
			{
				"address": dbus.MakeVariant("192.168.0.51"),
				"prefix":  dbus.MakeVariant(uint32(24)),
			},
		}),
		"Gateway": dbus.MakeVariant("192.168.0.1"),
		"NameserverData": dbus.MakeVariant([]map[string]dbus.Variant{
			{"address": dbus.MakeVariant("192.168.0.1")},
			{"address": dbus.MakeVariant("1.1.1.1")},
		}),
	}

	got := parseIPConfigProperties(props)
	want := model.IPConfig{
		Addresses: []model.IPAddress{
			{Address: "192.168.0.50", Prefix: 24},
			{Address: "192.168.0.51", Prefix: 24},
		},
		Gateway: "192.168.0.1",
		DNS:     []string{"192.168.0.1", "1.1.1.1"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseIPConfigProperties() = %#v, want %#v", got, want)
	}
}

func TestParseIPConfigPropertiesIgnoresIncompleteEntries(t *testing.T) {
	props := map[string]dbus.Variant{
		"AddressData": dbus.MakeVariant([]map[string]dbus.Variant{
			{"prefix": dbus.MakeVariant(uint32(24))},
		}),
		"NameserverData": dbus.MakeVariant([]map[string]dbus.Variant{
			{"address": dbus.MakeVariant("")},
		}),
	}

	got := parseIPConfigProperties(props)
	if len(got.Addresses) != 0 {
		t.Fatalf("expected no addresses, got %#v", got.Addresses)
	}
	if len(got.DNS) != 0 {
		t.Fatalf("expected no DNS servers, got %#v", got.DNS)
	}
}

func TestSSIDValuePreservesUsableText(t *testing.T) {
	values := map[string]dbus.Variant{
		"Ssid": dbus.MakeVariant([]byte("Home Wi-Fi")),
	}
	if got := ssidValue(values, "Ssid"); got != "Home Wi-Fi" {
		t.Fatalf("ssidValue() = %q, want Home Wi-Fi", got)
	}
}

func TestBoolValueDefault(t *testing.T) {
	values := map[string]dbus.Variant{}
	if got := boolValueDefault(values, "autoconnect", true); !got {
		t.Fatal("missing autoconnect should use the provided default")
	}

	values["autoconnect"] = dbus.MakeVariant(false)
	if got := boolValueDefault(values, "autoconnect", true); got {
		t.Fatal("explicit false autoconnect must be preserved")
	}
}
