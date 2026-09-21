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

// SnapshotSource is the read-only data contract consumed by the TUI.
type SnapshotSource interface {
	Snapshot(context.Context) (model.Snapshot, error)
}

type snapshotMsg struct {
	snapshot model.Snapshot
}

type snapshotErrMsg struct {
	err error
}

// Model is the read-only NetworkManager-HSP terminal interface.
type Model struct {
	source   SnapshotSource
	snapshot model.Snapshot
	width    int
	height   int
	cursor   int
	expanded bool
	loading  bool
	err      error
}

// New creates a read-only TUI model.
func New(source SnapshotSource) Model {
	return Model{
		source:  source,
		loading: true,
	}
}

// Init requests the first NetworkManager snapshot.
func (m Model) Init() tea.Cmd {
	return m.loadSnapshot()
}

// Update handles terminal input and refresh events. Checkpoint 3 contains no
// network-changing actions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.expanded = false
			}

		case "down", "j":
			if m.cursor+1 < len(m.visibleDevices()) {
				m.cursor++
				m.expanded = false
			}

		case "enter":
			if len(m.visibleDevices()) > 0 {
				m.expanded = !m.expanded
			}

		case "r":
			if !m.loading {
				m.loading = true
				m.err = nil
				return m, m.loadSnapshot()
			}
		}
	}

	return m, nil
}

// View renders the modern read-only interface in the alternate screen.
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

func (m *Model) clampCursor() {
	count := len(m.visibleDevices())
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

	contentWidth := width - 2
	var out strings.Builder

	header := titleStyle.Render("NetworkManager-HSP") + "  " +
		mutedStyle.Render("friendly network manager · read-only")
	out.WriteString(lipgloss.NewStyle().
		Width(contentWidth).
		Padding(0, 1).
		Render(header))
	out.WriteString("\n")
	out.WriteString(m.renderStatus(contentWidth))

	switch {
	case m.loading:
		out.WriteString("\n\n  " + warningStyle.Render("Refreshing network state..."))
	case m.err != nil:
		out.WriteString("\n\n  " + errorStyle.Render("Unable to read NetworkManager"))
		out.WriteString("\n  " + mutedStyle.Render(m.err.Error()))
	default:
		out.WriteString("\n")
		out.WriteString(m.renderDevices(contentWidth))
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

	networking := "Networking off"
	if m.snapshot.NetworkingEnabled {
		networking = "Networking on"
	}

	wireless := "Wi-Fi off"
	if m.snapshot.WirelessEnabled {
		wireless = "Wi-Fi on"
	}

	version := "NetworkManager " + emptyFallback(m.snapshot.Version, "unknown")
	line := fmt.Sprintf(
		"%s   %s   %s   %s",
		status,
		mutedStyle.Render(networking),
		mutedStyle.Render(wireless),
		mutedStyle.Render(version),
	)

	return cardStyle.
		Width(cardContentWidth(width)).
		Render(line)
}

func (m Model) renderDevices(width int) string {
	devices := m.visibleDevices()
	if len(devices) == 0 {
		return "\n" + cardStyle.
			Width(cardContentWidth(width)).
			Padding(1, 2).
			Render("No Ethernet or Wi-Fi devices detected.")
	}

	parts := make([]string, 0, len(devices))
	for index, device := range devices {
		parts = append(parts, m.renderDevice(width, device, index == m.cursor))
	}
	return "\n" + strings.Join(parts, "\n")
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
	help := "↑/↓ or j/k navigate   Enter details   r refresh   q/Esc exit"
	if width < 58 {
		help = "↑/↓ move  Enter details  r refresh  q exit"
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
	// Lip Gloss Width applies to the card content and border/padding are added
	// outside it. Leave room so cards fit the terminal width.
	if width <= 6 {
		return 1
	}
	return width - 6
}
