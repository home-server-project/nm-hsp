// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

type wifiSource interface {
	Snapshot(context.Context) (model.Snapshot, error)
	WiFiNetworks(context.Context, string) ([]model.WiFiNetwork, error)
	RequestWiFiScan(context.Context, string) error
	SetWirelessEnabled(context.Context, bool) error
	ActivateWiFiProfile(context.Context, string, string, string) error
	DisconnectWiFi(context.Context, string) error
	ForgetWiFiProfile(context.Context, string) error
	UpdateWiFiProfileMetadata(context.Context, model.WiFiProfileUpdate) error
	ConnectWiFi(context.Context, model.WiFiConnectRequest) (string, string, error)
}

type wifiRefreshMsg struct {
	snapshot model.Snapshot
	device   model.Device
	networks []model.WiFiNetwork
}

type wifiRefreshErrMsg struct {
	err error
}

type wifiOperationKind int

const (
	wifiOperationGeneric wifiOperationKind = iota
	wifiOperationConnect
	wifiOperationProfileSave
)

type wifiOperationMsg struct {
	kind   wifiOperationKind
	notice string
	err    error
}

type wifiListItem struct {
	network *model.WiFiNetwork
	profile *model.ConnectionProfile
}

type wifiMenuAction int

const (
	wifiMenuConnect wifiMenuAction = iota
	wifiMenuDisconnect
	wifiMenuEdit
	wifiMenuForget
	wifiMenuBack
)

type wifiMenuOption struct {
	label  string
	action wifiMenuAction
}

type wifiActionMenu struct {
	title   string
	options []wifiMenuOption
	cursor  int
}

type wifiScreen struct {
	source          wifiSource
	device          model.Device
	snapshot        model.Snapshot
	networks        []model.WiFiNetwork
	items           []wifiListItem
	cursor          int
	loading         bool
	busy            bool
	notice          string
	err             error
	menu            *wifiActionMenu
	connectForm     *wifiConnectForm
	profileForm     *wifiProfileForm
	confirmForget   *model.ConnectionProfile
	wirelessEnabled bool
}

func newWiFiScreen(source wifiSource, device model.Device, snapshot model.Snapshot) *wifiScreen {
	screen := &wifiScreen{
		source:          source,
		device:          device,
		snapshot:        snapshot,
		loading:         true,
		wirelessEnabled: snapshot.WirelessEnabled,
	}
	screen.rebuildItems()
	return screen
}

func (s *wifiScreen) init() tea.Cmd {
	return s.refresh(false)
}

func (s *wifiScreen) update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case wifiRefreshMsg:
		s.snapshot = msg.snapshot
		s.device = msg.device
		s.networks = msg.networks
		s.wirelessEnabled = msg.snapshot.WirelessEnabled
		s.loading = false
		s.busy = false
		s.err = nil
		s.rebuildItems()
		return false, nil

	case wifiRefreshErrMsg:
		s.loading = false
		s.busy = false
		s.err = msg.err
		return false, nil

	case wifiOperationMsg:
		s.busy = false
		if msg.err != nil {
			s.routeOperationError(msg)
			return false, nil
		}
		s.finishOperation(msg)
		s.loading = true
		return false, s.refresh(false)

	case tea.KeyPressMsg:
		if s.busy || s.loading {
			if msg.String() == "esc" && !s.busy {
				return true, nil
			}
			return false, nil
		}

		if s.connectForm != nil {
			return false, s.updateConnectForm(msg)
		}
		if s.profileForm != nil {
			return false, s.updateProfileForm(msg)
		}
		if s.confirmForget != nil {
			return false, s.updateForgetConfirmation(msg)
		}
		if s.menu != nil {
			return false, s.updateMenu(msg)
		}

		switch msg.String() {
		case "esc", "q":
			return true, nil
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor+1 < len(s.items) {
				s.cursor++
			}
		case "enter":
			if len(s.items) > 0 {
				s.openActionMenu(s.items[s.cursor])
			}
		case "r":
			if s.wirelessEnabled {
				s.loading = true
				s.notice = ""
				s.err = nil
				return false, s.refresh(true)
			}
		case "h":
			if s.wirelessEnabled {
				s.connectForm = newHiddenWiFiConnectForm(s.device.ObjectPath)
			}
		case "w":
			s.busy = true
			return false, s.setRadio(!s.wirelessEnabled)
		}
	}

	return false, nil
}

func (s *wifiScreen) updateConnectForm(msg tea.KeyPressMsg) tea.Cmd {
	action, cmd := s.connectForm.handleKey(msg)
	switch action {
	case wifiConnectActionCancel:
		s.connectForm.clearSecret()
		s.connectForm = nil
		return nil
	case wifiConnectActionSubmit:
		request, err := s.connectForm.buildRequest()
		if err != nil {
			s.connectForm.message = err.Error()
			s.connectForm.messageIsError = true
			return nil
		}
		s.busy = true
		return s.connect(request)
	default:
		return cmd
	}
}

func (s *wifiScreen) updateProfileForm(msg tea.KeyPressMsg) tea.Cmd {
	action, cmd := s.profileForm.handleKey(msg)
	switch action {
	case wifiProfileActionCancel:
		s.profileForm = nil
		return nil
	case wifiProfileActionSave:
		update, err := s.profileForm.buildUpdate()
		if err != nil {
			s.profileForm.message = err.Error()
			s.profileForm.messageIsError = true
			return nil
		}
		s.busy = true
		return s.saveProfile(update)
	default:
		return cmd
	}
}

func (s *wifiScreen) updateForgetConfirmation(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "y", "Y", "enter":
		profile := *s.confirmForget
		s.confirmForget = nil
		s.busy = true
		return s.forget(profile)
	case "n", "N", "esc":
		s.confirmForget = nil
	}
	return nil
}

func (s *wifiScreen) updateMenu(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		s.menu = nil
		return nil
	case "up", "k":
		if s.menu.cursor > 0 {
			s.menu.cursor--
		}
		return nil
	case "down", "j":
		if s.menu.cursor+1 < len(s.menu.options) {
			s.menu.cursor++
		}
		return nil
	case "enter":
		option := s.menu.options[s.menu.cursor]
		item := s.items[s.cursor]
		s.menu = nil
		return s.runMenuAction(option.action, item)
	}
	return nil
}

func (s *wifiScreen) runMenuAction(action wifiMenuAction, item wifiListItem) tea.Cmd {
	switch action {
	case wifiMenuBack:
		return nil
	case wifiMenuDisconnect:
		if s.device.ActiveConnectionPath == "" {
			s.err = fmt.Errorf("no active Wi-Fi connection is available to disconnect")
			return nil
		}
		s.busy = true
		return s.disconnect()
	case wifiMenuConnect:
		if item.profile != nil {
			s.busy = true
			accessPoint := ""
			if item.network != nil {
				accessPoint = item.network.ObjectPath
			}
			return s.activate(*item.profile, accessPoint)
		}
		if item.network == nil {
			return nil
		}
		if !supportedNewWiFiSecurity(item.network.Security) {
			s.err = fmt.Errorf("new %s Wi-Fi setup is not supported yet", item.network.Security)
			return nil
		}
		s.connectForm = newVisibleWiFiConnectForm(s.device.ObjectPath, *item.network)
		return nil
	case wifiMenuEdit:
		if item.profile != nil {
			s.profileForm = newWiFiProfileForm(*item.profile)
		}
		return nil
	case wifiMenuForget:
		if item.profile != nil {
			profile := *item.profile
			s.confirmForget = &profile
		}
		return nil
	default:
		return nil
	}
}

func (s *wifiScreen) openActionMenu(item wifiListItem) {
	options := make([]wifiMenuOption, 0, 4)
	active := item.network != nil && item.network.Active
	if !active && item.profile != nil && s.device.ActiveConnection != nil {
		active = item.profile.UUID == s.device.ActiveConnection.UUID
	}

	if active {
		options = append(options, wifiMenuOption{label: "Disconnect", action: wifiMenuDisconnect})
	} else if item.profile != nil || (item.network != nil && supportedNewWiFiSecurity(item.network.Security)) {
		options = append(options, wifiMenuOption{label: "Connect", action: wifiMenuConnect})
	}
	if item.profile != nil {
		options = append(options, wifiMenuOption{label: "Edit saved profile", action: wifiMenuEdit})
		if !active {
			options = append(options, wifiMenuOption{label: "Forget saved profile", action: wifiMenuForget})
		}
	}
	options = append(options, wifiMenuOption{label: "Back", action: wifiMenuBack})

	if len(options) == 1 && item.network != nil && !supportedNewWiFiSecurity(item.network.Security) {
		s.err = fmt.Errorf("new %s Wi-Fi setup is not supported yet", item.network.Security)
	}
	s.menu = &wifiActionMenu{title: s.itemName(item), options: options}
}

func (s *wifiScreen) refresh(rescan bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if rescan {
			if err := s.source.RequestWiFiScan(ctx, s.device.ObjectPath); err != nil {
				return wifiRefreshErrMsg{err: err}
			}
			time.Sleep(1200 * time.Millisecond)
		}

		snapshot, err := s.source.Snapshot(ctx)
		if err != nil {
			return wifiRefreshErrMsg{err: err}
		}
		device, ok := findDevice(snapshot.Devices, s.device.ObjectPath, s.device.Interface)
		if !ok {
			return wifiRefreshErrMsg{err: fmt.Errorf("Wi-Fi adapter %s is no longer available", s.device.Interface)}
		}

		var networks []model.WiFiNetwork
		if snapshot.WirelessEnabled {
			networks, err = s.source.WiFiNetworks(ctx, device.ObjectPath)
			if err != nil {
				return wifiRefreshErrMsg{err: err}
			}
		}
		return wifiRefreshMsg{snapshot: snapshot, device: device, networks: networks}
	}
}

func (s *wifiScreen) setRadio(enabled bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.source.SetWirelessEnabled(ctx, enabled); err != nil {
			return wifiOperationMsg{kind: wifiOperationGeneric, err: err}
		}
		state := "off"
		if enabled {
			state = "on"
		}
		return wifiOperationMsg{kind: wifiOperationGeneric, notice: "Wi-Fi radio turned " + state + "."}
	}
}

func (s *wifiScreen) activate(profile model.ConnectionProfile, accessPointPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := s.source.ActivateWiFiProfile(ctx, profile.ObjectPath, s.device.ObjectPath, accessPointPath); err != nil {
			return wifiOperationMsg{kind: wifiOperationGeneric, err: err}
		}
		return wifiOperationMsg{kind: wifiOperationGeneric, notice: "Connected using saved profile " + profile.ID + "."}
	}
}

func (s *wifiScreen) disconnect() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.source.DisconnectWiFi(ctx, s.device.ActiveConnectionPath); err != nil {
			return wifiOperationMsg{kind: wifiOperationGeneric, err: err}
		}
		return wifiOperationMsg{kind: wifiOperationGeneric, notice: "Wi-Fi disconnected."}
	}
}

func (s *wifiScreen) forget(profile model.ConnectionProfile) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.source.ForgetWiFiProfile(ctx, profile.ObjectPath); err != nil {
			return wifiOperationMsg{kind: wifiOperationGeneric, err: err}
		}
		return wifiOperationMsg{kind: wifiOperationGeneric, notice: "Forgot saved Wi-Fi profile " + profile.ID + "."}
	}
}

func (s *wifiScreen) saveProfile(update model.WiFiProfileUpdate) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.source.UpdateWiFiProfileMetadata(ctx, update); err != nil {
			return wifiOperationMsg{kind: wifiOperationProfileSave, err: err}
		}
		return wifiOperationMsg{kind: wifiOperationProfileSave, notice: "Saved Wi-Fi profile settings."}
	}
}

func (s *wifiScreen) connect(request model.WiFiConnectRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _, err := s.source.ConnectWiFi(ctx, request)
		if err != nil {
			return wifiOperationMsg{kind: wifiOperationConnect, err: err}
		}
		return wifiOperationMsg{kind: wifiOperationConnect, notice: "Connected to " + request.SSID + "."}
	}
}

func (s *wifiScreen) routeOperationError(msg wifiOperationMsg) {
	switch msg.kind {
	case wifiOperationConnect:
		if s.connectForm != nil {
			s.connectForm.message = msg.err.Error()
			s.connectForm.messageIsError = true
			return
		}
	case wifiOperationProfileSave:
		if s.profileForm != nil {
			s.profileForm.message = msg.err.Error()
			s.profileForm.messageIsError = true
			return
		}
	}
	s.err = msg.err
}

func (s *wifiScreen) finishOperation(msg wifiOperationMsg) {
	if msg.kind == wifiOperationConnect && s.connectForm != nil {
		s.connectForm.clearSecret()
		s.connectForm = nil
	}
	if msg.kind == wifiOperationProfileSave {
		s.profileForm = nil
	}
	s.notice = msg.notice
	s.err = nil
}

func (s *wifiScreen) rebuildItems() {
	items := make([]wifiListItem, 0, len(s.networks))
	represented := make(map[string]bool)

	for index := range s.networks {
		network := s.networks[index]
		if network.SSID == "" {
			continue
		}
		profile := s.profileForNetwork(network)
		if profile != nil {
			represented[profile.UUID] = true
		}
		networkCopy := network
		items = append(items, wifiListItem{network: &networkCopy, profile: profile})
	}

	for index := range s.snapshot.Profiles {
		profile := s.snapshot.Profiles[index]
		if profile.Type != "802-11-wireless" || profile.ObjectPath == "" || represented[profile.UUID] {
			continue
		}
		if profile.InterfaceName != "" && profile.InterfaceName != s.device.Interface {
			continue
		}
		profileCopy := profile
		items = append(items, wifiListItem{profile: &profileCopy})
	}

	s.items = items
	if len(items) == 0 {
		s.cursor = 0
	} else if s.cursor >= len(items) {
		s.cursor = len(items) - 1
	}
}

func (s *wifiScreen) profileForNetwork(network model.WiFiNetwork) *model.ConnectionProfile {
	var fallback *model.ConnectionProfile
	for index := range s.snapshot.Profiles {
		profile := s.snapshot.Profiles[index]
		if profile.Type != "802-11-wireless" || profile.SSID != network.SSID {
			continue
		}
		profileCopy := profile
		if profile.KeyManagement == network.KeyManagement {
			return &profileCopy
		}
		if fallback == nil || profile.AutoconnectPriority > fallback.AutoconnectPriority {
			fallback = &profileCopy
		}
	}
	return fallback
}

func (s *wifiScreen) render(width int) string {
	if width < 48 {
		width = 48
	}
	if s.connectForm != nil {
		return s.connectForm.render(width, s.busy)
	}
	if s.profileForm != nil {
		return s.profileForm.render(width, s.busy)
	}

	var out strings.Builder
	out.WriteString(titleStyle.Render("Wi-Fi · " + emptyFallback(s.device.Interface, "adapter")))
	radio := errorStyle.Render("Radio Off")
	if s.wirelessEnabled {
		radio = goodStyle.Render("Radio On")
	}
	out.WriteString("  " + radio)
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render("Visible networks and saved profiles"))
	out.WriteString("\n\n")

	switch {
	case s.loading:
		out.WriteString(warningStyle.Render("Refreshing Wi-Fi networks..."))
	case !s.wirelessEnabled:
		out.WriteString(cardStyle.Width(cardContentWidth(width)).Render("Wi-Fi is turned off. Press w to turn it on."))
	case len(s.items) == 0:
		out.WriteString(cardStyle.Width(cardContentWidth(width)).Render("No Wi-Fi networks or saved profiles found. Press r to rescan."))
	default:
		for index, item := range s.items {
			out.WriteString(s.renderItem(width, item, index == s.cursor))
			out.WriteString("\n")
		}
	}

	if s.menu != nil {
		out.WriteString("\n")
		out.WriteString(s.renderMenu(width))
	}
	if s.confirmForget != nil {
		out.WriteString("\n")
		out.WriteString(warningStyle.Render("Forget saved profile " + s.confirmForget.ID + "?  y/N"))
	}
	if s.err != nil {
		out.WriteString("\n" + errorStyle.Render(s.err.Error()))
	}
	if s.notice != "" {
		out.WriteString("\n" + goodStyle.Render(s.notice))
	}
	if s.busy {
		out.WriteString("\n" + warningStyle.Render("Working..."))
	}

	out.WriteString("\n\n")
	out.WriteString(helpStyle.Render("↑/↓ navigate   Enter actions   r rescan   h hidden network   w radio   Esc back"))
	return out.String()
}

func (s *wifiScreen) renderItem(width int, item wifiListItem, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}

	name := s.itemName(item)
	status := ""
	if item.network != nil && item.network.Active {
		status = goodStyle.Render("Connected")
	} else if item.profile != nil {
		status = mutedStyle.Render("Saved")
	}

	signal := "—"
	security := securityLabelForProfile(item.profile)
	band := ""
	if item.network != nil {
		signal = fmt.Sprintf("%d%%", item.network.Strength)
		security = string(item.network.Security)
		band = wifiBand(item.network.FrequencyMHz)
	}

	line := fmt.Sprintf("%s%s  %s\n    Signal %s   %s", marker, titleStyle.Render(name), status, signal, security)
	if band != "" {
		line += "   " + band
	}
	return style.Width(cardContentWidth(width)).Render(line)
}

func (s *wifiScreen) renderMenu(width int) string {
	var lines []string
	lines = append(lines, titleStyle.Render(s.menu.title))
	for index, option := range s.menu.options {
		marker := "  "
		if index == s.menu.cursor {
			marker = "› "
		}
		lines = append(lines, marker+option.label)
	}
	return selectedCardStyle.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n"))
}

func (s *wifiScreen) itemName(item wifiListItem) string {
	if item.network != nil && item.network.SSID != "" {
		return item.network.SSID
	}
	if item.profile != nil {
		return emptyFallback(item.profile.SSID, item.profile.ID)
	}
	return "Wi-Fi network"
}

func supportedNewWiFiSecurity(security model.WiFiSecurity) bool {
	switch security {
	case model.WiFiSecurityOpen,
		model.WiFiSecurityOWE,
		model.WiFiSecurityPersonal,
		model.WiFiSecurityWPA3Personal:
		return true
	default:
		return false
	}
}

func securityLabelForProfile(profile *model.ConnectionProfile) string {
	if profile == nil {
		return ""
	}
	switch profile.KeyManagement {
	case "":
		return string(model.WiFiSecurityOpen)
	case "owe":
		return string(model.WiFiSecurityOWE)
	case "wpa-psk":
		return string(model.WiFiSecurityPersonal)
	case "sae":
		return string(model.WiFiSecurityWPA3Personal)
	case "wpa-eap", "wpa-eap-suite-b-192", "ieee8021x":
		return string(model.WiFiSecurityEnterprise)
	case "none":
		return string(model.WiFiSecurityWEP)
	default:
		return string(model.WiFiSecurityUnknown)
	}
}

func wifiBand(frequency uint32) string {
	switch {
	case frequency == 0:
		return ""
	case frequency < 3000:
		return "2.4 GHz"
	case frequency < 5925:
		return "5 GHz"
	default:
		return "6 GHz"
	}
}

func findDevice(devices []model.Device, objectPath, interfaceName string) (model.Device, bool) {
	for _, device := range devices {
		if objectPath != "" && device.ObjectPath == objectPath {
			return device, true
		}
	}
	for _, device := range devices {
		if interfaceName != "" && device.Interface == interfaceName {
			return device, true
		}
	}
	return model.Device{}, false
}
