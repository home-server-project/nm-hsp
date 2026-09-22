// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
	"github.com/home-server-project/nm-hsp/internal/security"
)

type wifiSource interface {
	Snapshot(context.Context) (model.Snapshot, error)
	WiFiNetworks(context.Context, string) ([]model.WiFiNetwork, error)
	RequestWiFiScan(context.Context, string) error
	SetWirelessEnabled(context.Context, bool) error
	ActivateWiFiProfile(context.Context, string, string, string) error
	DisconnectWiFi(context.Context, string) error
	ForgetWiFiProfile(context.Context, string, string) error
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
	item    wifiListItem
	options []wifiMenuOption
	cursor  int
}

type wifiScreen struct {
	source          wifiSource
	device          model.Device
	snapshot        model.Snapshot
	networks        []model.WiFiNetwork
	items           []wifiListItem
	savedItems      []wifiListItem
	cursor          int
	savedCursor     int
	savedOpen       bool
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
		if s.savedOpen {
			return false, s.updateSavedNetworks(msg)
		}

		switch msg.String() {
		case "esc", "q":
			return true, nil
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor+1 < s.mainItemCount() {
				s.cursor++
			}
		case "enter":
			if s.cursor == 0 {
				s.savedOpen = true
				s.err = nil
				s.notice = ""
				return false, nil
			}
			if item, ok := s.mainSelectedItem(); ok {
				s.openActionMenu(item)
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

func (s *wifiScreen) updateSavedNetworks(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q":
		s.savedOpen = false
		s.err = nil
		return nil
	case "up", "k":
		if s.savedCursor > 0 {
			s.savedCursor--
		}
	case "down", "j":
		if s.savedCursor+1 < len(s.savedItems) {
			s.savedCursor++
		}
	case "enter":
		if len(s.savedItems) > 0 {
			s.openActionMenu(s.savedItems[s.savedCursor])
		}
	case "r":
		s.loading = true
		s.notice = ""
		s.err = nil
		return s.refresh(true)
	}
	return nil
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
		s.connectForm.clearSecret()
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
		item := s.menu.item
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
		if s.device.ObjectPath == "" {
			s.err = fmt.Errorf("no Wi-Fi device is available to disconnect")
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
		options = append(options, wifiMenuOption{label: "Forget saved profile", action: wifiMenuForget})
	}
	options = append(options, wifiMenuOption{label: "Back", action: wifiMenuBack})

	if len(options) == 1 && item.network != nil && !supportedNewWiFiSecurity(item.network.Security) {
		s.err = fmt.Errorf("new %s Wi-Fi setup is not supported yet", item.network.Security)
	}
	s.menu = &wifiActionMenu{
		title:   s.itemName(item),
		item:    item,
		options: options,
	}
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
		if err := s.source.DisconnectWiFi(ctx, s.device.ObjectPath); err != nil {
			return wifiOperationMsg{kind: wifiOperationGeneric, err: err}
		}
		return wifiOperationMsg{kind: wifiOperationGeneric, notice: "Wi-Fi disconnected."}
	}
}

func (s *wifiScreen) forget(profile model.ConnectionProfile) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.source.ForgetWiFiProfile(ctx, profile.ObjectPath, s.device.ObjectPath); err != nil {
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
		defer request.Password.Clear()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _, err := s.source.ConnectWiFi(ctx, request)
		if err != nil {
			return wifiOperationMsg{
				kind: wifiOperationConnect,
				err:  security.RedactError(err, request.Password),
			}
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
	visible := make([]wifiListItem, 0, len(s.networks))
	for index := range s.networks {
		network := s.networks[index]
		if network.SSID == "" {
			continue
		}
		networkCopy := network
		visible = append(visible, wifiListItem{
			network: &networkCopy,
			profile: s.profileForNetwork(network),
		})
	}
	sort.SliceStable(visible, func(i, j int) bool {
		leftActive := visible[i].network != nil && visible[i].network.Active
		rightActive := visible[j].network != nil && visible[j].network.Active
		if leftActive != rightActive {
			return leftActive
		}
		leftStrength := uint8(0)
		rightStrength := uint8(0)
		if visible[i].network != nil {
			leftStrength = visible[i].network.Strength
		}
		if visible[j].network != nil {
			rightStrength = visible[j].network.Strength
		}
		if leftStrength != rightStrength {
			return leftStrength > rightStrength
		}
		return s.itemName(visible[i]) < s.itemName(visible[j])
	})

	saved := make([]wifiListItem, 0)
	for index := range s.snapshot.Profiles {
		profile := s.snapshot.Profiles[index]
		if profile.Type != "802-11-wireless" || profile.ObjectPath == "" {
			continue
		}
		if profile.InterfaceName != "" && profile.InterfaceName != s.device.Interface {
			continue
		}
		profileCopy := profile
		saved = append(saved, wifiListItem{
			network: s.networkForProfile(profile),
			profile: &profileCopy,
		})
	}
	sort.SliceStable(saved, func(i, j int) bool {
		leftActive := s.itemActive(saved[i])
		rightActive := s.itemActive(saved[j])
		if leftActive != rightActive {
			return leftActive
		}
		return strings.ToLower(s.itemName(saved[i])) < strings.ToLower(s.itemName(saved[j]))
	})

	s.items = visible
	s.savedItems = saved
	s.clampWiFiCursors()
}

func (s *wifiScreen) clampWiFiCursors() {
	if s.mainItemCount() == 0 {
		s.cursor = 0
	} else if s.cursor >= s.mainItemCount() {
		s.cursor = s.mainItemCount() - 1
	}
	if len(s.savedItems) == 0 {
		s.savedCursor = 0
	} else if s.savedCursor >= len(s.savedItems) {
		s.savedCursor = len(s.savedItems) - 1
	}
}

func (s *wifiScreen) mainItemCount() int {
	return 1 + len(s.items)
}

func (s *wifiScreen) mainSelectedItem() (wifiListItem, bool) {
	index := s.cursor - 1
	if index < 0 || index >= len(s.items) {
		return wifiListItem{}, false
	}
	return s.items[index], true
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

func (s *wifiScreen) networkForProfile(profile model.ConnectionProfile) *model.WiFiNetwork {
	var fallback *model.WiFiNetwork
	for index := range s.networks {
		network := s.networks[index]
		if network.SSID != profile.SSID {
			continue
		}
		networkCopy := network
		if network.KeyManagement == profile.KeyManagement {
			return &networkCopy
		}
		if fallback == nil || network.Strength > fallback.Strength {
			fallback = &networkCopy
		}
	}
	return fallback
}

func (s *wifiScreen) render(width, height int) string {
	if width < 48 {
		width = 48
	}
	if height <= 0 {
		height = 24
	}
	if s.connectForm != nil {
		return s.connectForm.render(width, s.busy)
	}
	if s.profileForm != nil {
		return s.profileForm.render(width, s.busy)
	}
	if s.confirmForget != nil {
		return s.renderForgetConfirmation(width)
	}
	if s.menu != nil {
		return s.renderMenuModal(width)
	}
	if s.savedOpen {
		return s.renderSavedNetworks(width, height)
	}

	return s.renderNearbyNetworks(width, height)
}

func (s *wifiScreen) renderNearbyNetworks(width, height int) string {
	var out strings.Builder
	out.WriteString(s.renderWiFiHeader())
	out.WriteString("\n")
	out.WriteString(mutedStyle.Render("Nearby networks"))
	out.WriteString("\n\n")

	switch {
	case s.loading:
		out.WriteString(warningStyle.Render("Refreshing Wi-Fi networks..."))
	case !s.wirelessEnabled:
		out.WriteString(cardStyle.Width(cardContentWidth(width)).Render("Wi-Fi is turned off. Press w to turn it on."))
	default:
		start, end := s.mainVisibleRange(height)
		for index := start; index < end; index++ {
			if index == 0 {
				out.WriteString(s.renderSavedEntry(width, s.cursor == 0))
			} else {
				out.WriteString(s.renderItem(width, s.items[index-1], index == s.cursor))
			}
			out.WriteString("\n")
		}
		if end-start < s.mainItemCount() {
			out.WriteString(mutedStyle.Render(fmt.Sprintf(
				"Showing %d-%d of %d entries",
				start+1,
				end,
				s.mainItemCount(),
			)))
			out.WriteString("\n")
		}
	}

	s.renderWiFiMessages(&out)
	out.WriteString("\n")
	out.WriteString(helpStyle.Render("↑/↓ navigate   Enter select   r rescan   h hidden network   w radio   Esc back"))
	return out.String()
}

func (s *wifiScreen) renderSavedNetworks(width, height int) string {
	var out strings.Builder
	out.WriteString(s.renderWiFiHeader())
	out.WriteString("\n")
	out.WriteString(titleStyle.Render("Saved networks"))
	out.WriteString("  ")
	out.WriteString(mutedStyle.Render(fmt.Sprintf("%d profiles", len(s.savedItems))))
	out.WriteString("\n\n")

	switch {
	case s.loading:
		out.WriteString(warningStyle.Render("Refreshing saved networks..."))
	case len(s.savedItems) == 0:
		out.WriteString(cardStyle.Width(cardContentWidth(width)).Render("No saved Wi-Fi profiles for this adapter."))
	default:
		start, end := s.savedVisibleRange(height)
		for index := start; index < end; index++ {
			out.WriteString(s.renderItem(width, s.savedItems[index], index == s.savedCursor))
			out.WriteString("\n")
		}
		if end-start < len(s.savedItems) {
			out.WriteString(mutedStyle.Render(fmt.Sprintf(
				"Showing %d-%d of %d saved networks",
				start+1,
				end,
				len(s.savedItems),
			)))
			out.WriteString("\n")
		}
	}

	s.renderWiFiMessages(&out)
	out.WriteString("\n")
	out.WriteString(helpStyle.Render("↑/↓ navigate   Enter actions   r refresh   Esc back"))
	return out.String()
}

func (s *wifiScreen) renderWiFiHeader() string {
	radio := errorStyle.Render("Radio Off")
	if s.wirelessEnabled {
		radio = goodStyle.Render("Radio On")
	}
	return titleStyle.Render("Wi-Fi · "+emptyFallback(s.device.Interface, "adapter")) + "  " + radio
}

func (s *wifiScreen) renderSavedEntry(width int, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}
	line := fmt.Sprintf(
		"%s%s\n    %d saved profiles · Enter to manage",
		marker,
		titleStyle.Render("Saved networks"),
		len(s.savedItems),
	)
	return style.Width(cardContentWidth(width)).Render(line)
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
	if s.itemActive(item) {
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

func (s *wifiScreen) renderMenuModal(width int) string {
	var out strings.Builder
	out.WriteString(s.renderWiFiHeader())
	out.WriteString("\n\n")
	out.WriteString(titleStyle.Render("Actions · " + s.menu.title))
	out.WriteString("\n\n")
	out.WriteString(s.renderMenu(width))
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Render("↑/↓ choose   Enter select   Esc cancel"))
	return out.String()
}

func (s *wifiScreen) renderMenu(width int) string {
	var lines []string
	for index, option := range s.menu.options {
		marker := "  "
		if index == s.menu.cursor {
			marker = "› "
		}
		lines = append(lines, marker+option.label)
	}
	return selectedCardStyle.Width(cardContentWidth(width)).Render(strings.Join(lines, "\n"))
}

func (s *wifiScreen) renderForgetConfirmation(width int) string {
	var out strings.Builder
	out.WriteString(s.renderWiFiHeader())
	out.WriteString("\n\n")
	out.WriteString(titleStyle.Render("Forget saved network"))
	out.WriteString("\n\n")
	message := "Forget saved profile " + s.confirmForget.ID + "?"
	if s.device.ActiveConnection != nil && s.device.ActiveConnection.UUID == s.confirmForget.UUID {
		message = "Disconnect and forget saved profile " + s.confirmForget.ID + "?"
	}
	out.WriteString(warningStyle.Render(message))
	out.WriteString("\n\n")
	out.WriteString(helpStyle.Render("Enter / y forget   Esc / n cancel"))
	return selectedCardStyle.Width(cardContentWidth(width)).Render(out.String())
}

func (s *wifiScreen) renderWiFiMessages(out *strings.Builder) {
	if s.err != nil {
		out.WriteString("\n" + errorStyle.Render(s.err.Error()))
	}
	if s.notice != "" {
		out.WriteString("\n" + goodStyle.Render(s.notice))
	}
	if s.busy {
		out.WriteString("\n" + warningStyle.Render("Working..."))
	}
}

func (s *wifiScreen) mainVisibleRange(height int) (int, int) {
	return wifiVisibleRange(s.mainItemCount(), s.cursor, height)
}

func (s *wifiScreen) savedVisibleRange(height int) (int, int) {
	return wifiVisibleRange(len(s.savedItems), s.savedCursor, height)
}

func wifiVisibleRange(total, cursor, height int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	maxVisible := (height - 8) / 4
	if maxVisible < 1 {
		maxVisible = 1
	}
	if maxVisible > total {
		maxVisible = total
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	start := cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	if start+maxVisible > total {
		start = total - maxVisible
	}
	return start, start + maxVisible
}

func (s *wifiScreen) itemActive(item wifiListItem) bool {
	if item.network != nil && item.network.Active {
		return true
	}
	return item.profile != nil &&
		s.device.ActiveConnection != nil &&
		item.profile.UUID != "" &&
		item.profile.UUID == s.device.ActiveConnection.UUID
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
