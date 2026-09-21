// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

func TestWiFiPasswordMaskedAndToggleable(t *testing.T) {
	form := newVisibleWiFiConnectForm("/device", model.WiFiNetwork{
		ObjectPath:    "/ap",
		SSID:          "Home Wi-Fi",
		Strength:      80,
		Security:      model.WiFiSecurityPersonal,
		KeyManagement: "wpa-psk",
	})
	form.password.SetValue("correct-horse")

	if form.password.EchoMode != textinput.EchoPassword {
		t.Fatal("Wi-Fi password must be masked by default")
	}

	form.field = wifiConnectShowPassword
	_, _ = form.handleKey(tea.KeyPressMsg(tea.Key{Code: ' '}))
	if form.password.EchoMode != textinput.EchoNormal {
		t.Fatal("show-password toggle should reveal the password only on request")
	}

	form.clearSecret()
	if form.password.Value() != "" {
		t.Fatal("clearSecret() must remove the transient password")
	}
}

func TestHiddenWiFiFormBuildsRequest(t *testing.T) {
	form := newHiddenWiFiConnectForm("/device")
	form.ssid.SetValue("HiddenNet")
	form.password.SetValue("correct-horse")
	form.priority.SetValue("15")

	request, err := form.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if !request.Hidden || request.SSID != "HiddenNet" || request.KeyManagement != "wpa-psk" {
		t.Fatalf("hidden request = %#v", request)
	}
	if request.AutoconnectPriority != 15 {
		t.Fatalf("priority = %d, want 15", request.AutoconnectPriority)
	}
}

func TestSavedWiFiProfileDoesNotExposePassword(t *testing.T) {
	form := newWiFiProfileForm(model.ConnectionProfile{
		ObjectPath:  "/profile",
		ID:          "Home",
		SSID:        "Home Wi-Fi",
		Autoconnect: true,
	})
	output := form.render(90, false)
	if !strings.Contains(output, "Password: unchanged") {
		t.Fatal("saved profile form must state that the password is unchanged")
	}
}

func TestWiFiActionMenuForSavedNetwork(t *testing.T) {
	screen := &wifiScreen{
		device:   model.Device{Interface: "wlan0"},
		snapshot: model.Snapshot{},
	}
	item := wifiListItem{
		network: &model.WiFiNetwork{
			SSID:          "Home Wi-Fi",
			Security:      model.WiFiSecurityPersonal,
			KeyManagement: "wpa-psk",
		},
		profile: &model.ConnectionProfile{
			ObjectPath:    "/profile",
			ID:            "Home Wi-Fi",
			SSID:          "Home Wi-Fi",
			Type:          "802-11-wireless",
			KeyManagement: "wpa-psk",
		},
	}
	screen.openActionMenu(item)

	var labels []string
	for _, option := range screen.menu.options {
		labels = append(labels, option.label)
	}
	joined := strings.Join(labels, "|")
	for _, want := range []string{"Connect", "Edit saved profile", "Forget saved profile", "Back"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("saved Wi-Fi actions missing %q: %s", want, joined)
		}
	}
}

func TestUnsupportedNewEnterpriseDoesNotOfferConnect(t *testing.T) {
	screen := &wifiScreen{device: model.Device{Interface: "wlan0"}}
	item := wifiListItem{
		network: &model.WiFiNetwork{
			SSID:          "Corp",
			Security:      model.WiFiSecurityEnterprise,
			KeyManagement: "wpa-eap",
		},
	}
	screen.openActionMenu(item)
	if screen.err == nil {
		t.Fatal("new Enterprise Wi-Fi should be reported as unsupported")
	}
	for _, option := range screen.menu.options {
		if option.action == wifiMenuConnect {
			t.Fatal("unsupported new Enterprise Wi-Fi must not offer Connect")
		}
	}
}

func TestOutOfRangeSavedWiFiProfileIsListed(t *testing.T) {
	screen := &wifiScreen{
		device: model.Device{Interface: "wlan0"},
		snapshot: model.Snapshot{
			Profiles: []model.ConnectionProfile{
				{
					ObjectPath:    "/profile",
					ID:            "Hidden Home",
					SSID:          "Hidden Home",
					Type:          "802-11-wireless",
					KeyManagement: "wpa-psk",
				},
			},
		},
	}
	screen.rebuildItems()
	if len(screen.items) != 1 || screen.items[0].profile == nil {
		t.Fatalf("saved out-of-range profile not listed: %#v", screen.items)
	}
}
