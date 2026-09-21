// SPDX-License-Identifier: Apache-2.0

package networkmanager

import (
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestClassifyWiFiSecurity(t *testing.T) {
	tests := []struct {
		name     string
		flags    uint32
		wpa      uint32
		rsn      uint32
		security model.WiFiSecurity
		keyMgmt  string
	}{
		{name: "open", security: model.WiFiSecurityOpen},
		{name: "wep", flags: apFlagPrivacy, security: model.WiFiSecurityWEP, keyMgmt: "none"},
		{name: "wpa personal", wpa: apSecKeyMgmtPSK, security: model.WiFiSecurityPersonal, keyMgmt: "wpa-psk"},
		{name: "wpa2 personal", rsn: apSecKeyMgmtPSK, security: model.WiFiSecurityPersonal, keyMgmt: "wpa-psk"},
		{name: "wpa3 only", rsn: apSecKeyMgmtSAE, security: model.WiFiSecurityWPA3Personal, keyMgmt: "sae"},
		{name: "transition personal", rsn: apSecKeyMgmtSAE | apSecKeyMgmtPSK, security: model.WiFiSecurityPersonal, keyMgmt: "wpa-psk"},
		{name: "owe", rsn: apSecKeyMgmtOWE, security: model.WiFiSecurityOWE, keyMgmt: "owe"},
		{name: "enterprise", rsn: apSecKeyMgmt8021X, security: model.WiFiSecurityEnterprise, keyMgmt: "wpa-eap"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			security, keyMgmt := classifyWiFiSecurity(tt.flags, tt.wpa, tt.rsn)
			if security != tt.security || keyMgmt != tt.keyMgmt {
				t.Fatalf("classifyWiFiSecurity() = %q, %q; want %q, %q", security, keyMgmt, tt.security, tt.keyMgmt)
			}
		})
	}
}
