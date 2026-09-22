// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestSameHardwareAddressNormalizesCaseAndSeparators(t *testing.T) {
	if !sameHardwareAddress("90:8d:6e:8a:c0:b1", "90-8D-6E-8A-C0-B1") {
		t.Fatal("sameHardwareAddress() should normalize case and separators")
	}
	if sameHardwareAddress("90:8D:6E:8A:C0:B1", "00:11:22:33:44:55") {
		t.Fatal("different hardware addresses must not match")
	}
	if sameHardwareAddress("", "90:8D:6E:8A:C0:B1") {
		t.Fatal("empty hardware address must not match")
	}
}

func TestRepairInterfaceBindingRequiresMACEvidence(t *testing.T) {
	settings := map[string]map[string]dbus.Variant{
		"connection": {
			"id":             dbus.MakeVariant("Wired"),
			"type":           dbus.MakeVariant("802-3-ethernet"),
			"interface-name": dbus.MakeVariant("enp1s0"),
		},
		"802-3-ethernet": {
			"mac-address": dbus.MakeVariant([]byte{0x90, 0x8d, 0x6e, 0x8a, 0xc0, 0xb1}),
		},
	}
	if got := hardwareAddressValue(settings["802-3-ethernet"], "mac-address"); got != "90:8D:6E:8A:C0:B1" {
		t.Fatalf("profile MAC = %q", got)
	}
	if !sameHardwareAddress(
		hardwareAddressValue(settings["802-3-ethernet"], "mac-address"),
		"90:8D:6E:8A:C0:B1",
	) {
		t.Fatal("verified profile/device MAC pair should be eligible for interface repair")
	}

	delete(settings["802-3-ethernet"], "mac-address")
	if sameHardwareAddress(
		hardwareAddressValue(settings["802-3-ethernet"], "mac-address"),
		"90:8D:6E:8A:C0:B1",
	) {
		t.Fatal("missing profile MAC must not be considered safe evidence")
	}
}

func TestDiagnosticsDHCPRepairProfileIsAutomatic(t *testing.T) {
	action := model.RepairAction{
		Kind:          model.RepairCreateDHCPProfile,
		DevicePath:    "/device/1",
		InterfaceName: "enp2s0",
	}
	profile := model.EthernetProfile{
		DevicePath:    action.DevicePath,
		InterfaceName: action.InterfaceName,
		Autoconnect:   true,
		IPv4:          model.IPProfileConfig{Method: model.IPMethodAuto},
		IPv6:          model.IPProfileConfig{Method: model.IPMethodAuto},
	}
	if profile.InterfaceName != "enp2s0" || !profile.Autoconnect {
		t.Fatalf("unexpected repair profile: %#v", profile)
	}
	if profile.IPv4.Method != model.IPMethodAuto || profile.IPv6.Method != model.IPMethodAuto {
		t.Fatalf("repair profile must use automatic IP methods: %#v", profile)
	}
}
