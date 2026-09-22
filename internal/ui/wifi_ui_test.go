// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/security"
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

	output := form.render(90, false)
	if strings.Contains(output, "correct-horse") {
		t.Fatal("masked Wi-Fi form render exposed the password")
	}
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
	if form.showPassword || form.password.EchoMode != textinput.EchoPassword {
		t.Fatal("clearSecret() must restore masked password mode")
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
	defer request.Password.Clear()
	if request.AutoconnectPriority != 15 {
		t.Fatalf("priority = %d, want 15", request.AutoconnectPriority)
	}
	if request.Password.Empty() {
		t.Fatal("personal Wi-Fi request should carry a transient secret")
	}
	if strings.Contains(fmt.Sprintf("%+v", request), "correct-horse") {
		t.Fatal("formatted Wi-Fi request exposed the password")
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

func TestOutOfRangeSavedWiFiProfileIsSeparatedFromNearbyNetworks(t *testing.T) {
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
	if len(screen.items) != 0 {
		t.Fatalf("out-of-range saved profile leaked into nearby networks: %#v", screen.items)
	}
	if len(screen.savedItems) != 1 || screen.savedItems[0].profile == nil {
		t.Fatalf("saved profile missing from saved-networks view: %#v", screen.savedItems)
	}
}

func TestWiFiSubmitClearsVisiblePasswordImmediately(t *testing.T) {
	form := newVisibleWiFiConnectForm("/device", model.WiFiNetwork{
		ObjectPath:    "/ap",
		SSID:          "Home Wi-Fi",
		Strength:      80,
		Security:      model.WiFiSecurityPersonal,
		KeyManagement: "wpa-psk",
	})
	form.password.SetValue("correct-horse")
	form.showPassword = true
	form.syncPasswordEcho()
	form.field = wifiConnectSubmit

	screen := &wifiScreen{connectForm: form}
	cmd := screen.updateConnectForm(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("Connect submission should return a command")
	}
	if !screen.busy {
		t.Fatal("Connect submission should mark the Wi-Fi screen busy")
	}
	if form.password.Value() != "" {
		t.Fatal("Connect submission must clear the visible password immediately")
	}
	if form.showPassword || form.password.EchoMode != textinput.EchoPassword {
		t.Fatal("Connect submission must restore masked password mode")
	}
}

func TestWiFiConnectCommandClearsSecretAndRedactsBackendError(t *testing.T) {
	password := security.NewSecret("correct-horse")
	request := model.WiFiConnectRequest{
		DevicePath:    "/device",
		SSID:          "Home Wi-Fi",
		KeyManagement: "wpa-psk",
		Password:      password,
	}
	source := &fakeSource{
		wifiConnectErr: errors.New("backend echoed password correct-horse"),
	}
	screen := &wifiScreen{source: source}

	msg := screen.connect(request)()
	operation, ok := msg.(wifiOperationMsg)
	if !ok {
		t.Fatalf("connect command returned %T, want wifiOperationMsg", msg)
	}
	if operation.err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(operation.err.Error(), "correct-horse") {
		t.Fatalf("UI connection error leaked password: %q", operation.err)
	}
	if !strings.Contains(operation.err.Error(), "[REDACTED]") {
		t.Fatalf("UI connection error missing redaction marker: %q", operation.err)
	}
	if !request.Password.Empty() {
		t.Fatal("UI-owned request secret should be empty after command completion")
	}
	if !source.wifiRequest.Password.Empty() {
		t.Fatal("backend request copy should observe the same cleared secret state")
	}
}

func TestWiFiMainViewStartsWithSavedNetworksEntry(t *testing.T) {
	screen := &wifiScreen{
		device:          model.Device{Interface: "wlan0"},
		wirelessEnabled: true,
		snapshot: model.Snapshot{
			Profiles: []model.ConnectionProfile{
				{
					ObjectPath: "/profile",
					ID:         "Old Network",
					SSID:       "Old Network",
					Type:       "802-11-wireless",
				},
			},
		},
		networks: []model.WiFiNetwork{
			{
				ObjectPath: "/ap/current",
				SSID:       "Current",
				Active:     true,
				Strength:   80,
				Security:   model.WiFiSecurityPersonal,
			},
		},
	}
	screen.rebuildItems()

	output := screen.render(90, 24)
	if !strings.Contains(output, "Saved networks") {
		t.Fatal("nearby view should start with a Saved networks entry")
	}
	if strings.Contains(output, "Old Network") {
		t.Fatal("out-of-range saved profile should not fill the nearby-networks view")
	}
	if !strings.Contains(output, "Current") {
		t.Fatal("visible current network missing from nearby-networks view")
	}
}

func TestWiFiConnectedNetworkSortsFirst(t *testing.T) {
	screen := &wifiScreen{
		device: model.Device{Interface: "wlan0"},
		networks: []model.WiFiNetwork{
			{SSID: "Strong", Strength: 100},
			{SSID: "Current", Strength: 20, Active: true},
		},
	}
	screen.rebuildItems()
	if len(screen.items) != 2 || screen.items[0].network == nil || screen.items[0].network.SSID != "Current" {
		t.Fatalf("connected network should be first: %#v", screen.items)
	}
}

func TestWiFiActionMenuRendersAsModal(t *testing.T) {
	screen := &wifiScreen{
		device:          model.Device{Interface: "wlan0"},
		wirelessEnabled: true,
		networks: []model.WiFiNetwork{
			{SSID: "One"},
			{SSID: "Two"},
			{SSID: "Three"},
		},
	}
	screen.rebuildItems()
	screen.openActionMenu(screen.items[0])

	output := screen.render(90, 18)
	if !strings.Contains(output, "Actions · One") {
		t.Fatal("action menu title should be visible immediately")
	}
	if strings.Contains(output, "Three") {
		t.Fatal("modal action view should not render the network list behind or below it")
	}

	_, _ = screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	if screen.menu != nil {
		t.Fatal("Esc should close the Wi-Fi action modal")
	}
}

func TestWiFiSavedNetworksEntryOpensDedicatedView(t *testing.T) {
	screen := &wifiScreen{
		device:          model.Device{Interface: "wlan0"},
		wirelessEnabled: true,
		snapshot: model.Snapshot{
			Profiles: []model.ConnectionProfile{
				{
					ObjectPath: "/profile",
					ID:         "Home",
					SSID:       "Home",
					Type:       "802-11-wireless",
				},
			},
		},
	}
	screen.rebuildItems()

	_, _ = screen.update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if !screen.savedOpen {
		t.Fatal("Enter on Saved networks should open the dedicated saved-profile view")
	}
	output := screen.render(90, 24)
	if !strings.Contains(output, "Saved networks") || !strings.Contains(output, "Home") {
		t.Fatalf("saved-networks view missing expected content: %q", output)
	}
}

func TestWiFiScrollingKeepsSelectedEntryVisible(t *testing.T) {
	screen := &wifiScreen{
		device:          model.Device{Interface: "wlan0"},
		wirelessEnabled: true,
	}
	for index := 0; index < 20; index++ {
		screen.networks = append(screen.networks, model.WiFiNetwork{
			SSID:     fmt.Sprintf("Network-%02d", index),
			Strength: uint8(100 - index),
		})
	}
	screen.rebuildItems()
	screen.cursor = screen.mainItemCount() - 1

	output := screen.render(90, 18)
	selected := screen.items[len(screen.items)-1].network.SSID
	if !strings.Contains(output, selected) {
		t.Fatalf("selected network %q should stay visible in a short terminal", selected)
	}
}
