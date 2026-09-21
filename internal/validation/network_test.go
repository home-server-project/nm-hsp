// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestParseAddresses(t *testing.T) {
	got, err := ParseAddresses("192.168.0.59/24, 10.10.10.2/16", false)
	if err != nil {
		t.Fatalf("ParseAddresses() error = %v", err)
	}
	if len(got) != 2 || got[0].Address != "192.168.0.59" || got[0].Prefix != 24 {
		t.Fatalf("ParseAddresses() = %#v", got)
	}
}

func TestParseAddressesRejectsWrongFamily(t *testing.T) {
	if _, err := ParseAddresses("2001:db8::10/64", false); err == nil {
		t.Fatal("expected IPv6 address in IPv4 field to fail")
	}
}

func TestParseGatewayAndDNS(t *testing.T) {
	gateway, err := ParseGateway("192.168.0.1", false)
	if err != nil || gateway != "192.168.0.1" {
		t.Fatalf("ParseGateway() = %q, %v", gateway, err)
	}

	dns, err := ParseDNS("1.1.1.1, 9.9.9.9", false)
	if err != nil || len(dns) != 2 {
		t.Fatalf("ParseDNS() = %#v, %v", dns, err)
	}
}

func TestParseMTUEnforcesIPv6Minimum(t *testing.T) {
	if _, err := ParseMTU("9000", true); err != nil {
		t.Fatalf("9000 should be valid: %v", err)
	}
	if _, err := ParseMTU("576", true); err == nil {
		t.Fatal("expected IPv6-enabled MTU below 1280 to fail")
	}
	if got, err := ParseMTU("", true); err != nil || got != 0 {
		t.Fatalf("blank MTU = %d, %v; want automatic", got, err)
	}
}

func TestValidateIPConfigManualRequiresAddress(t *testing.T) {
	config := model.IPProfileConfig{Method: model.IPMethodManual}
	if err := ValidateIPConfig(config, false); err == nil {
		t.Fatal("manual mode without address should fail")
	}
}

func TestValidateEthernetProfileAcceptsNormalConfiguration(t *testing.T) {
	profile := model.EthernetProfile{
		ProfilePath:   "/org/freedesktop/NetworkManager/Settings/1",
		InterfaceName: "enp2s0",
		Autoconnect:   true,
		MTU:           1500,
		IPv4: model.IPProfileConfig{
			Method: model.IPMethodManual,
			Addresses: []model.IPAddress{
				{Address: "192.168.0.59", Prefix: 24},
			},
			Gateway: "192.168.0.1",
			DNS:     []string{"192.168.0.1", "1.1.1.1"},
		},
		IPv6: model.IPProfileConfig{
			Method: model.IPMethodAuto,
			DNS:    []string{"2606:4700:4700::1111"},
		},
	}

	if err := ValidateEthernetProfile(profile); err != nil {
		t.Fatalf("ValidateEthernetProfile() error = %v", err)
	}
}
