// SPDX-License-Identifier: Apache-2.0

package diagnostics

import (
	"strings"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

func carrier(value bool) *bool {
	return &value
}

func baseEthernetSnapshot() model.Snapshot {
	return model.Snapshot{
		NetworkingEnabled: true,
		Connectivity:      4,
		Devices: []model.Device{
			{
				ObjectPath:      "/device/1",
				Interface:       "enp2s0",
				Kind:            model.DeviceKindEthernet,
				Managed:         true,
				State:           30,
				Carrier:         carrier(true),
				SpeedMbps:       1000,
				HardwareAddress: "90:8D:6E:8A:C0:B1",
			},
		},
	}
}

func findCheck(report model.DiagnosticReport, id string) *model.DiagnosticCheck {
	for index := range report.Checks {
		if report.Checks[index].ID == id {
			return &report.Checks[index]
		}
	}
	return nil
}

func TestMissingEthernetProfileOffersFreshDHCPProfile(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	report := Analyze(snapshot, model.DiagnosticProbe{})

	check := findCheck(report, "ethernet-profile-missing")
	if check == nil {
		t.Fatal("missing Ethernet profile was not diagnosed")
	}
	if check.Repair == nil || check.Repair.Kind != model.RepairCreateDHCPProfile {
		t.Fatalf("repair = %#v, want create DHCP profile", check.Repair)
	}
	if check.Repair.InterfaceName != "enp2s0" {
		t.Fatalf("repair interface = %q", check.Repair.InterfaceName)
	}
}

func TestAutoconnectDisabledOffersNarrowRepair(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	profile := model.ConnectionProfile{
		ObjectPath:    "/profile/1",
		ID:            "Wired",
		UUID:          "uuid-1",
		Type:          "802-3-ethernet",
		InterfaceName: "enp2s0",
		IPv4Method:    "auto",
		Autoconnect:   false,
	}
	snapshot.Profiles = []model.ConnectionProfile{profile}
	snapshot.Devices[0].AvailableProfileUUIDs = []string{"uuid-1"}

	report := Analyze(snapshot, model.DiagnosticProbe{})
	check := findCheck(report, "profile-autoconnect-disabled")
	if check == nil || check.Repair == nil {
		t.Fatalf("autoconnect repair missing: %#v", check)
	}
	if check.Repair.Kind != model.RepairEnableAutoconnect {
		t.Fatalf("repair kind = %q", check.Repair.Kind)
	}
}

func TestStaleInterfaceBindingNeedsMatchingMACForRepair(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	profile := model.ConnectionProfile{
		ObjectPath:      "/profile/1",
		ID:              "Wired",
		UUID:            "uuid-1",
		Type:            "802-3-ethernet",
		InterfaceName:   "enp1s0",
		WiredMACAddress: "90:8D:6E:8A:C0:B1",
		IPv4Method:      "auto",
		Autoconnect:     true,
	}
	snapshot.Profiles = []model.ConnectionProfile{profile}

	report := Analyze(snapshot, model.DiagnosticProbe{})
	check := findCheck(report, "ethernet-stale-interface-binding")
	if check == nil || check.Repair == nil {
		t.Fatalf("safe stale-interface repair missing: %#v", check)
	}
	if check.Repair.Before != "enp1s0" || check.Repair.After != "enp2s0" {
		t.Fatalf("repair before/after = %#v", check.Repair)
	}

	snapshot.Profiles[0].WiredMACAddress = ""
	report = Analyze(snapshot, model.DiagnosticProbe{})
	if check := findCheck(report, "ethernet-stale-interface-binding"); check != nil && check.Repair != nil {
		t.Fatal("interface repair must not be offered without matching MAC evidence")
	}
}

func TestMismatchedMACIsDiagnosedWithoutAutomaticRepair(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	snapshot.Profiles = []model.ConnectionProfile{
		{
			ObjectPath:      "/profile/1",
			ID:              "Wired",
			UUID:            "uuid-1",
			Type:            "802-3-ethernet",
			InterfaceName:   "enp2s0",
			WiredMACAddress: "00:11:22:33:44:55",
			IPv4Method:      "auto",
			Autoconnect:     true,
		},
	}
	report := Analyze(snapshot, model.DiagnosticProbe{})
	check := findCheck(report, "ethernet-mac-mismatch")
	if check == nil {
		t.Fatal("MAC mismatch was not diagnosed")
	}
	if check.Repair != nil {
		t.Fatal("MAC mismatch must not be auto-repaired")
	}
}

func TestDHCPWithoutAddressOffersRetryOnly(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	active := model.ConnectionProfile{
		ObjectPath:    "/profile/1",
		ID:            "Wired",
		UUID:          "uuid-1",
		Type:          "802-3-ethernet",
		InterfaceName: "enp2s0",
		IPv4Method:    "auto",
		Autoconnect:   true,
	}
	snapshot.Profiles = []model.ConnectionProfile{active}
	snapshot.Devices[0].ActiveConnection = &active
	snapshot.Devices[0].State = 100

	report := Analyze(snapshot, model.DiagnosticProbe{})
	check := findCheck(report, "ethernet-dhcp-no-address")
	if check == nil || check.Repair == nil || check.Repair.Kind != model.RepairActivateProfile {
		t.Fatalf("DHCP retry repair missing: %#v", check)
	}
}

func TestAddressWithoutGatewayIsDiagnosedButNotGuessed(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	active := model.ConnectionProfile{
		ObjectPath:    "/profile/1",
		ID:            "Static",
		Type:          "802-3-ethernet",
		InterfaceName: "enp2s0",
		IPv4Method:    "manual",
		Autoconnect:   true,
	}
	snapshot.Devices[0].ActiveConnection = &active
	snapshot.Devices[0].IPv4.Addresses = []model.IPAddress{{Address: "192.168.0.50", Prefix: 24}}

	report := Analyze(snapshot, model.DiagnosticProbe{})
	check := findCheck(report, "ethernet-no-gateway")
	if check == nil {
		t.Fatal("missing gateway was not diagnosed")
	}
	if check.Repair != nil {
		t.Fatal("nm-hsp must not guess a gateway")
	}
}

func TestDNSAndInternetFailuresAreSeparated(t *testing.T) {
	snapshot := baseEthernetSnapshot()
	active := model.ConnectionProfile{
		ObjectPath:    "/profile/1",
		ID:            "Wired",
		Type:          "802-3-ethernet",
		InterfaceName: "enp2s0",
		IPv4Method:    "auto",
		Autoconnect:   true,
	}
	snapshot.Devices[0].ActiveConnection = &active
	snapshot.Devices[0].IPv4 = model.IPConfig{
		Addresses: []model.IPAddress{{Address: "192.168.0.50", Prefix: 24}},
		Gateway:   "192.168.0.1",
		DNS:       []string{"192.168.0.1"},
	}
	snapshot.Connectivity = 1

	report := Analyze(snapshot, model.DiagnosticProbe{
		DNSAttempted: true,
		DNSWorking:   true,
	})
	internet := findCheck(report, "internet-none")
	if internet == nil || !strings.Contains(internet.Detail, "DNS lookup works") {
		t.Fatalf("DNS-working/no-Internet distinction missing: %#v", internet)
	}

	report = Analyze(snapshot, model.DiagnosticProbe{
		DNSAttempted: true,
		DNSWorking:   false,
		DNSError:     "lookup failed",
	})
	dns := findCheck(report, "dns-lookup-failed")
	if dns == nil || !strings.Contains(dns.Detail, "lookup failed") {
		t.Fatalf("DNS failure distinction missing: %#v", dns)
	}
}

func TestWiFiRadioDisabledOffersOnlyRadioRepair(t *testing.T) {
	snapshot := model.Snapshot{
		NetworkingEnabled: true,
		WirelessEnabled:   false,
		Devices: []model.Device{
			{
				ObjectPath: "/device/wifi",
				Interface:  "wlan0",
				Kind:       model.DeviceKindWiFi,
				Managed:    true,
				State:      30,
			},
		},
	}
	report := Analyze(snapshot, model.DiagnosticProbe{})
	check := findCheck(report, "wifi-radio-disabled")
	if check == nil || check.Repair == nil || check.Repair.Kind != model.RepairEnableWiFi {
		t.Fatalf("Wi-Fi radio repair missing: %#v", check)
	}
}

func TestNoAdapterExplainsDriverBoundary(t *testing.T) {
	report := Analyze(model.Snapshot{NetworkingEnabled: true}, model.DiagnosticProbe{})
	check := findCheck(report, "no-supported-adapters")
	if check == nil || !strings.Contains(check.Detail, "does not install hardware drivers") {
		t.Fatalf("driver boundary missing: %#v", check)
	}
}
