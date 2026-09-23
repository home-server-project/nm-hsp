// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/home-server-project/nm-hsp/internal/model"
)

// NetworkSource is the NetworkManager data contract consumed by the TUI.
type NetworkSource interface {
	wifiSource
	EthernetProfile(context.Context, string, string) (model.EthernetProfile, error)
	SaveEthernetProfile(context.Context, model.EthernetProfile) (model.EthernetProfile, error)
	ActivateEthernetProfile(context.Context, string, string) error
	DisconnectEthernet(context.Context, string) error
	ApplyRepair(context.Context, model.RepairAction) error
	VPNAction(context.Context, model.VPNProviderID, model.VPNAction) (model.VPNActionResult, error)
	VPNWaitAuthentication(context.Context, model.VPNProviderID, string) (model.VPNActionResult, error)
}

type snapshotMsg struct {
	snapshot model.Snapshot
}

type snapshotErrMsg struct {
	err error
}

type ethernetProfileMsg struct {
	profile model.EthernetProfile
}

type ethernetProfileErrMsg struct {
	err error
}

type ethernetSavedMsg struct {
	profile model.EthernetProfile
}

type ethernetSaveErrMsg struct {
	err error
}

// Model is the NetworkManager-HSP terminal interface.
type Model struct {
	source       NetworkSource
	snapshot     model.Snapshot
	width        int
	height       int
	cursor       int
	expanded     bool
	loading      bool
	formLoading  bool
	formSaving   bool
	form         *ethernetForm
	formBackMenu *ethernetActionMenu
	ethernetMenu *ethernetActionMenu
	wifi         *wifiScreen
	vpn          *privateAccessScreen
	diagnostics  *diagnosticsScreen
	err          error
	notice       string
}

// New creates the TUI model using the built-in dark-terminal theme.
func New(source NetworkSource) Model {
	return NewWithTheme(source, ThemeDark)
}

// NewWithTheme creates the TUI model using the selected built-in terminal theme.
func NewWithTheme(source NetworkSource, mode ThemeMode) Model {
	ApplyTheme(mode)
	return Model{
		source:  source,
		loading: true,
	}
}

// Init requests the first NetworkManager snapshot.
func (m Model) Init() tea.Cmd {
	return m.loadSnapshot()
}

// Update handles dashboard navigation plus Ethernet and Wi-Fi management.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.diagnostics != nil {
		closeScreen, cmd := m.diagnostics.update(msg)
		if closeScreen {
			jumpWiFi := m.diagnostics.jumpWiFi
			jumpSnapshot := m.diagnostics.snapshot
			m.diagnostics = nil
			m.err = nil
			if jumpWiFi != nil {
				m.snapshot = jumpSnapshot
				m.loading = false
				m.wifi = newWiFiScreen(m.source, *jumpWiFi, jumpSnapshot)
				return m, m.wifi.init()
			}
			m.loading = true
			return m, m.loadSnapshot()
		}
		return m, cmd
	}
	if m.vpn != nil {
		closeScreen, cmd := m.vpn.update(msg)
		if closeScreen {
			m.snapshot = m.vpn.snapshot
			m.vpn = nil
			m.err = nil
			m.clampCursor()
		}
		return m, cmd
	}
	if m.ethernetMenu != nil {
		closeMenu, cmd := m.updateEthernetMenu(msg)
		if closeMenu {
			m.ethernetMenu = nil
		}
		return m, cmd
	}
	if m.wifi != nil {
		closeScreen, cmd := m.wifi.update(msg)
		if closeScreen {
			m.wifi = nil
			m.loading = true
			m.err = nil
			return m, m.loadSnapshot()
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case snapshotMsg:
		m.snapshot = msg.snapshot
		m.loading = false
		m.err = nil
		m.clampCursor()

	case snapshotErrMsg:
		m.loading = false
		m.err = msg.err

	case ethernetProfileMsg:
		m.formLoading = false
		m.form = newEthernetForm(msg.profile)
		m.err = nil

	case ethernetProfileErrMsg:
		m.formLoading = false
		m.err = msg.err

	case ethernetSavedMsg:
		m.formSaving = false
		m.form = nil
		m.notice = "Ethernet profile saved. Live networking was not restarted."
		m.err = nil
		m.loading = true
		return m, m.loadSnapshot()

	case ethernetSaveErrMsg:
		m.formSaving = false
		if m.form != nil {
			m.form.message = msg.err.Error()
			m.form.messageIsError = true
		}

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.form != nil {
			if m.formSaving {
				return m, nil
			}

			action, cmd := m.form.handleKey(msg)
			if msg.String() == "q" {
				action = formActionCancel
				cmd = nil
			}
			switch action {
			case formActionCancel:
				m.form = nil
				m.err = nil
				if m.formBackMenu != nil {
					m.ethernetMenu = m.formBackMenu
					m.formBackMenu = nil
				}
				return m, nil

			case formActionSave:
				profile, err := m.form.buildProfile()
				if err != nil {
					m.form.message = err.Error()
					m.form.messageIsError = true
					return m, nil
				}
				m.formSaving = true
				return m, m.saveEthernetProfile(profile)
			}
			return m, cmd
		}

		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit

		case "up", "k":
			m.notice = ""
			if m.cursor > 0 {
				m.cursor--
				m.expanded = false
			}

		case "down", "j":
			m.notice = ""
			if m.cursor+1 < m.dashboardItemCount() {
				m.cursor++
				m.expanded = false
			}

		case "enter":
			m.notice = ""
			devices := m.visibleDevices()
			if m.cursor == len(devices) {
				m.vpn = newPrivateAccessScreen(m.source, m.snapshot)
				return m, nil
			}
			if m.cursor == len(devices)+1 {
				m.diagnostics = newDiagnosticsScreen(m.source)
				return m, m.diagnostics.init()
			}
			if len(devices) == 0 || m.cursor >= len(devices) {
				break
			}

			device := devices[m.cursor]
			if device.Kind == model.DeviceKindEthernet {
				m.ethernetMenu = newEthernetActionMenu(device, m.preferredEthernetProfile(device))
				return m, nil
			}
			if device.Kind == model.DeviceKindWiFi {
				m.wifi = newWiFiScreen(m.source, device, m.snapshot)
				return m, m.wifi.init()
			}
			m.expanded = !m.expanded

		case "t":
			m.notice = ""
			m.diagnostics = newDiagnosticsScreen(m.source)
			return m, m.diagnostics.init()

		case "d":
			m.notice = ""
			if m.cursor < len(m.visibleDevices()) {
				m.expanded = !m.expanded
			}

		case "r":
			m.notice = ""
			if !m.loading && !m.formLoading {
				m.loading = true
				m.err = nil
				return m, m.loadSnapshot()
			}
		}
	}

	return m, nil
}

// View renders the interface in the alternate screen.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	return view
}

func (m Model) loadSnapshot() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		snapshot, err := m.source.Snapshot(ctx)
		if err != nil {
			return snapshotErrMsg{err: err}
		}
		return snapshotMsg{snapshot: snapshot}
	}
}

func (m Model) loadEthernetProfile(profilePath, devicePath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		profile, err := m.source.EthernetProfile(ctx, profilePath, devicePath)
		if err != nil {
			return ethernetProfileErrMsg{err: err}
		}
		return ethernetProfileMsg{profile: profile}
	}
}

func (m Model) saveEthernetProfile(profile model.EthernetProfile) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		saved, err := m.source.SaveEthernetProfile(ctx, profile)
		if err != nil {
			return ethernetSaveErrMsg{err: err}
		}
		return ethernetSavedMsg{profile: saved}
	}
}

func (m *Model) openEthernetForm(device model.Device) tea.Cmd {
	m.err = nil
	m.expanded = false

	if profile := m.preferredEthernetProfile(device); profile != nil {
		m.formLoading = true
		return m.loadEthernetProfile(profile.ObjectPath, device.ObjectPath)
	}

	m.form = newEthernetForm(model.EthernetProfile{
		DevicePath:    device.ObjectPath,
		InterfaceName: device.Interface,
		Autoconnect:   true,
		IPv4: model.IPProfileConfig{
			Method: model.IPMethodAuto,
		},
		IPv6: model.IPProfileConfig{
			Method: model.IPMethodAuto,
		},
	})
	return nil
}

func (m Model) preferredEthernetProfile(device model.Device) *model.ConnectionProfile {
	if device.ActiveConnection != nil &&
		device.ActiveConnection.Type == "802-3-ethernet" &&
		device.ActiveConnection.ObjectPath != "" {
		profile := *device.ActiveConnection
		return &profile
	}

	available := make(map[string]struct{}, len(device.AvailableProfileUUIDs))
	for _, uuid := range device.AvailableProfileUUIDs {
		available[uuid] = struct{}{}
	}

	candidates := make([]model.ConnectionProfile, 0)
	for _, profile := range m.snapshot.Profiles {
		if profile.Type != "802-3-ethernet" || profile.ObjectPath == "" {
			continue
		}
		if len(available) > 0 {
			if _, ok := available[profile.UUID]; !ok {
				continue
			}
		} else if profile.InterfaceName != "" && profile.InterfaceName != device.Interface {
			continue
		}
		candidates = append(candidates, profile)
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].AutoconnectPriority != candidates[j].AutoconnectPriority {
			return candidates[i].AutoconnectPriority > candidates[j].AutoconnectPriority
		}
		if candidates[i].Autoconnect != candidates[j].Autoconnect {
			return candidates[i].Autoconnect
		}
		return candidates[i].ID < candidates[j].ID
	})

	profile := candidates[0]
	return &profile
}

func (m *Model) clampCursor() {
	count := m.dashboardItemCount()
	if count == 0 {
		m.cursor = 0
		m.expanded = false
		return
	}
	if m.cursor >= count {
		m.cursor = count - 1
		m.expanded = false
	}
}

func (m Model) dashboardItemCount() int {
	return len(m.visibleDevices()) + 2
}

func (m Model) visibleDevices() []model.Device {
	devices := make([]model.Device, 0, len(m.snapshot.Devices))
	for _, device := range m.snapshot.Devices {
		if device.Kind == model.DeviceKindEthernet || device.Kind == model.DeviceKindWiFi {
			devices = append(devices, device)
		}
	}

	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Kind == devices[j].Kind {
			return devices[i].Interface < devices[j].Interface
		}
		return devices[i].Kind == model.DeviceKindEthernet
	})

	return devices
}

func (m Model) render() string {
	width := m.width
	if width <= 0 {
		width = 88
	}
	if width < 32 {
		width = 32
	}

	if m.vpn != nil {
		return m.vpn.render(width-2, m.height)
	}
	if m.diagnostics != nil {
		return m.diagnostics.render(width-2, m.height)
	}
	if m.wifi != nil {
		return m.wifi.render(width-2, m.height)
	}
	if m.ethernetMenu != nil {
		return m.renderEthernetMenu(width - 2)
	}
	if m.form != nil {
		return m.form.render(width-2, m.formSaving)
	}

	contentWidth := width - 2
	var out strings.Builder

	header := titleStyle.Render("NetworkManager-HSP") + "  " +
		mutedStyle.Render("friendly network manager from Home Server Project")
	out.WriteString(lipgloss.NewStyle().
		Width(contentWidth).
		Padding(0, 1).
		Render(header))
	out.WriteString("\n")
	out.WriteString(lipgloss.NewStyle().
		Width(contentWidth).
		Padding(0, 1).
		Render(mutedStyle.Render("https://github.com/home-server-project")))
	out.WriteString("\n")
	out.WriteString(m.renderStatus(contentWidth))

	switch {
	case m.formLoading:
		out.WriteString("\n\n  " + warningStyle.Render("Loading Ethernet settings..."))
	case m.loading:
		out.WriteString("\n\n  " + warningStyle.Render("Refreshing network state..."))
	case m.err != nil:
		out.WriteString("\n\n  " + errorStyle.Render("NetworkManager operation failed"))
		out.WriteString("\n  " + mutedStyle.Render(m.err.Error()))
	default:
		out.WriteString("\n")
		out.WriteString(m.renderDevices(contentWidth))
	}

	if m.notice != "" {
		out.WriteString("\n  " + goodStyle.Render(m.notice))
	}

	out.WriteString("\n")
	out.WriteString(m.renderHelp(contentWidth))
	return out.String()
}

func (m Model) renderStatus(width int) string {
	connectivity := model.ConnectivityName(m.snapshot.Connectivity)
	status := mutedStyle.Render("Connectivity unknown")

	switch connectivity {
	case "full":
		status = goodStyle.Render("Internet connected")
	case "limited":
		status = warningStyle.Render("Limited connectivity")
	case "portal":
		status = warningStyle.Render("Captive portal")
	case "none":
		status = errorStyle.Render("No connectivity")
	}

	second := ""
	if !m.snapshot.NetworkingEnabled {
		second = mutedStyle.Render("Networking ") + errorStyle.Render("OFF")
	} else {
		ethernetConnected := hasActivatedDevice(m.snapshot.Devices, model.DeviceKindEthernet)
		ethernetState := warningStyle.Render("disconnected")
		if ethernetConnected {
			ethernetState = goodStyle.Render("connected")
		}
		second = mutedStyle.Render("Ethernet ") + ethernetState
	}

	wifiState := warningStyle.Render("disconnected")
	if !m.snapshot.WirelessEnabled {
		wifiState = errorStyle.Render("OFF")
	} else if hasActivatedDevice(m.snapshot.Devices, model.DeviceKindWiFi) {
		wifiState = goodStyle.Render("connected")
	}
	wireless := mutedStyle.Render("Wi-Fi ") + wifiState
	version := mutedStyle.Render("NetworkManager " + emptyFallback(m.snapshot.Version, "unknown"))
	line := fmt.Sprintf(
		"%s   %s   %s   %s",
		status,
		second,
		wireless,
		version,
	)

	return cardStyle.
		Width(cardContentWidth(width)).
		Render(line)
}

func hasActivatedDevice(devices []model.Device, kind model.DeviceKind) bool {
	for _, device := range devices {
		if device.Kind == kind && device.State == 100 && device.ActiveConnection != nil {
			return true
		}
	}
	return false
}

func (m Model) renderDevices(width int) string {
	devices := m.visibleDevices()
	parts := make([]string, 0, len(devices)+2)
	if len(devices) == 0 {
		parts = append(parts, cardStyle.
			Width(cardContentWidth(width)).
			Padding(1, 2).
			Render("No Ethernet or Wi-Fi devices detected."))
	} else {
		for index, device := range devices {
			parts = append(parts, m.renderDevice(width, device, index == m.cursor))
		}
	}
	parts = append(parts, m.renderPrivateAccessCard(width, m.cursor == len(devices)))
	parts = append(parts, m.renderTroubleshootCard(width, m.cursor == len(devices)+1))
	return "\n" + strings.Join(parts, "\n")
}

func (m Model) renderPrivateAccessCard(width int, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}
	body := fmt.Sprintf(
		"%s%s\n    %s",
		marker,
		titleStyle.Render("Private Access"),
		mutedStyle.Render(privateAccessSummary(m.snapshot)),
	)
	return style.Width(cardContentWidth(width)).Render(body)
}

func (m Model) renderTroubleshootCard(width int, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}
	body := fmt.Sprintf(
		"%s%s\n    %s",
		marker,
		titleStyle.Render("Troubleshoot"),
		mutedStyle.Render("Network health, diagnostics, and safe repair"),
	)
	return style.Width(cardContentWidth(width)).Render(body)
}

func (m Model) renderDevice(width int, device model.Device, selected bool) string {
	style := cardStyle
	marker := "  "
	if selected {
		style = selectedCardStyle
		marker = "› "
	}

	label := "Ethernet"
	if device.Kind == model.DeviceKindWiFi {
		label = "Wi-Fi"
	}

	state := renderDeviceState(device.State)
	ipv4 := emptyFallback(firstAddress(device.IPv4), "—")
	profile := "No active profile"
	if device.ActiveConnection != nil && device.ActiveConnection.ID != "" {
		profile = device.ActiveConnection.ID
	}

	var summary string
	if width >= 76 {
		summary = fmt.Sprintf(
			"%s%s  %s\n    %s   IPv4 %s   %s",
			marker,
			titleStyle.Render(label+" · "+emptyFallback(device.Interface, "unknown")),
			state,
			mutedStyle.Render(profile),
			ipv4,
			mutedStyle.Render(deviceExtra(device)),
		)
	} else {
		summary = fmt.Sprintf(
			"%s%s  %s\n    %s\n    IPv4 %s\n    %s",
			marker,
			titleStyle.Render(label+" · "+emptyFallback(device.Interface, "unknown")),
			state,
			mutedStyle.Render(profile),
			ipv4,
			mutedStyle.Render(deviceExtra(device)),
		)
	}

	if selected && m.expanded {
		summary += "\n" + m.renderDetails(device)
	}

	return style.
		Width(cardContentWidth(width)).
		Render(summary)
}

func (m Model) renderDetails(device model.Device) string {
	lines := []string{
		"    MAC       " + emptyFallback(device.HardwareAddress, "—"),
		fmt.Sprintf("    MTU       %d", device.MTU),
		"    IPv4      " + addresses(device.IPv4),
		"    Gateway   " + emptyFallback(device.IPv4.Gateway, "—"),
		"    DNS       " + listOrDash(device.IPv4.DNS),
		"    IPv6      " + addresses(device.IPv6),
	}

	if device.ActiveConnection != nil {
		lines = append(
			lines,
			fmt.Sprintf("    Autoconnect %t", device.ActiveConnection.Autoconnect),
		)
	}

	return mutedStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) renderHelp(width int) string {
	help := "↑/↓ or j/k navigate   Enter select   t troubleshoot   d details   r refresh   q exit"
	if width < 68 {
		help = "↑/↓ move   Enter select   t troubleshoot   r refresh   q exit"
	}
	return helpStyle.
		Width(width).
		Render(help)
}

func renderDeviceState(state uint32) string {
	switch state {
	case 100:
		return goodStyle.Render("connected")
	case 120:
		return errorStyle.Render("failed")
	case 30:
		return warningStyle.Render("disconnected")
	default:
		return mutedStyle.Render(model.DeviceStateName(state))
	}
}

func deviceExtra(device model.Device) string {
	switch device.Kind {
	case model.DeviceKindWiFi:
		if device.Wireless == nil {
			return ""
		}
		if device.Wireless.SSID != "" {
			return fmt.Sprintf("%s · %d%% signal", device.Wireless.SSID, device.Wireless.Signal)
		}
		return "not associated"

	case model.DeviceKindEthernet:
		if device.Carrier != nil && !*device.Carrier {
			return "cable disconnected"
		}
		if device.SpeedMbps > 0 {
			return fmt.Sprintf("%d Mbps", device.SpeedMbps)
		}
	}
	return ""
}

func firstAddress(config model.IPConfig) string {
	if len(config.Addresses) == 0 {
		return ""
	}
	address := config.Addresses[0]
	return fmt.Sprintf("%s/%d", address.Address, address.Prefix)
}

func addresses(config model.IPConfig) string {
	if len(config.Addresses) == 0 {
		return "—"
	}

	values := make([]string, 0, len(config.Addresses))
	for _, address := range config.Addresses {
		values = append(values, fmt.Sprintf("%s/%d", address.Address, address.Prefix))
	}
	return strings.Join(values, ", ")
}

func listOrDash(values []string) string {
	if len(values) == 0 {
		return "—"
	}
	return strings.Join(values, ", ")
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cardContentWidth(width int) int {
	if width <= 6 {
		return 1
	}
	return width - 6
}
