// SPDX-License-Identifier: Apache-2.0

package diagnostics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/home-server-project/nm-hsp/internal/model"
)

// Analyze converts a NetworkManager snapshot and optional active probe results
// into a friendly, deterministic health report.
func Analyze(snapshot model.Snapshot, probe model.DiagnosticProbe) model.DiagnosticReport {
	report := model.DiagnosticReport{}

	supported := 0
	for _, device := range snapshot.Devices {
		if device.Kind == model.DeviceKindEthernet || device.Kind == model.DeviceKindWiFi {
			supported++
		}
	}
	if supported == 0 {
		report.Checks = append(report.Checks, model.DiagnosticCheck{
			ID:     "no-supported-adapters",
			Status: model.DiagnosticProblem,
			Title:  "No Ethernet or Wi-Fi adapter detected",
			Detail: "NetworkManager is not exposing a supported network adapter. nm-hsp can diagnose this state but does not install hardware drivers.",
		})
	}

	if !snapshot.NetworkingEnabled {
		report.Checks = append(report.Checks, model.DiagnosticCheck{
			ID:     "networking-disabled",
			Status: model.DiagnosticProblem,
			Title:  "Networking is turned off",
			Detail: "NetworkManager has global networking disabled.",
		})
	}

	for _, device := range snapshot.Devices {
		switch device.Kind {
		case model.DeviceKindEthernet:
			report.Checks = append(report.Checks, analyzeEthernet(snapshot, device)...)
		case model.DeviceKindWiFi:
			report.Checks = append(report.Checks, analyzeWiFi(snapshot, device)...)
		}
	}

	report.Checks = append(report.Checks, analyzeDNS(snapshot, probe)...)
	report.Checks = append(report.Checks, analyzeConnectivity(snapshot, probe))
	return report
}

func analyzeEthernet(snapshot model.Snapshot, device model.Device) []model.DiagnosticCheck {
	checks := make([]model.DiagnosticCheck, 0, 8)
	base := checkBase(device)

	if !device.Managed {
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "ethernet-unmanaged",
			Status: model.DiagnosticProblem,
			Title:  "Ethernet adapter is not managed",
			Detail: "NetworkManager is not managing this adapter. nm-hsp will not change management policy automatically.",
		}))
		return checks
	}
	checks = append(checks, withBase(base, model.DiagnosticCheck{
		ID:     "ethernet-managed",
		Status: model.DiagnosticOK,
		Title:  "Ethernet adapter is managed",
		Detail: "NetworkManager can configure this adapter.",
	}))

	carrierUp := device.Carrier != nil && *device.Carrier
	switch {
	case device.Carrier == nil:
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "ethernet-carrier-unknown",
			Status: model.DiagnosticInfo,
			Title:  "Cable state is unavailable",
			Detail: "NetworkManager did not report Ethernet carrier state.",
		}))
	case carrierUp:
		detail := "Physical Ethernet link is detected."
		if device.SpeedMbps > 0 {
			detail = fmt.Sprintf("Physical Ethernet link is detected at %d Mbps.", device.SpeedMbps)
		}
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "ethernet-carrier-up",
			Status: model.DiagnosticOK,
			Title:  "Cable link detected",
			Detail: detail,
		}))
	default:
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "ethernet-carrier-down",
			Status: model.DiagnosticProblem,
			Title:  "Ethernet cable is disconnected",
			Detail: "The adapter is present, but no physical carrier is detected. Check the cable, switch port, or upstream device.",
		}))
	}

	profiles := ethernetProfiles(snapshot.Profiles)
	candidate := bestEthernetProfile(device, profiles)
	macMismatch := ethernetMACMismatchProfile(device, profiles)
	if device.ActiveConnection != nil && device.ActiveConnection.Type == "802-3-ethernet" {
		active := *device.ActiveConnection
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "ethernet-profile-active",
			Status: model.DiagnosticOK,
			Title:  "Ethernet profile is active",
			Detail: fmt.Sprintf("NetworkManager is using %q.", active.ID),
		}))
		if !active.Autoconnect {
			checks = append(checks, autoconnectCheck(device, active))
		}
		candidate = &active
	} else if candidate != nil {
		if candidate.InterfaceName != "" && candidate.InterfaceName != device.Interface {
			if sameMAC(candidate.WiredMACAddress, device.HardwareAddress) {
				checks = append(checks, withBase(base, model.DiagnosticCheck{
					ID:     "ethernet-stale-interface-binding",
					Status: model.DiagnosticProblem,
					Title:  "Saved profile is bound to an old interface name",
					Detail: fmt.Sprintf(
						"%q is bound to %q, but its hardware address matches %s.",
						candidate.ID,
						candidate.InterfaceName,
						device.Interface,
					),
					Repair: &model.RepairAction{
						Kind:          model.RepairFixInterfaceBinding,
						Label:         "Use this adapter",
						Description:   "Change only the saved interface-name binding. IP settings and other profile options stay unchanged.",
						DevicePath:    device.ObjectPath,
						InterfaceName: device.Interface,
						ProfilePath:   candidate.ObjectPath,
						ProfileID:     candidate.ID,
						Before:        candidate.InterfaceName,
						After:         device.Interface,
					},
				}))
			} else {
				checks = append(checks, withBase(base, model.DiagnosticCheck{
					ID:     "ethernet-interface-mismatch",
					Status: model.DiagnosticWarning,
					Title:  "Saved profile targets another interface",
					Detail: fmt.Sprintf("%q is bound to %q, not %q. nm-hsp will not change this automatically without a matching hardware address.", candidate.ID, candidate.InterfaceName, device.Interface),
				}))
			}
		} else {
			checks = append(checks, withBase(base, model.DiagnosticCheck{
				ID:     "ethernet-profile-inactive",
				Status: model.DiagnosticWarning,
				Title:  "Saved Ethernet profile is not active",
				Detail: fmt.Sprintf("%q is available but is not currently connected.", candidate.ID),
				Repair: &model.RepairAction{
					Kind:          model.RepairActivateProfile,
					Label:         "Activate profile",
					Description:   "Ask NetworkManager to activate this existing saved profile without changing its IP settings.",
					DevicePath:    device.ObjectPath,
					InterfaceName: device.Interface,
					ProfilePath:   candidate.ObjectPath,
					ProfileID:     candidate.ID,
					Before:        "inactive",
					After:         "active",
				},
			}))
			if !candidate.Autoconnect {
				checks = append(checks, autoconnectCheck(device, *candidate))
			}
		}
	} else {
		unmatched := unmatchedEthernetProfiles(device, profiles)
		detail := "No persistent Ethernet connection is configured for this adapter."
		if len(unmatched) > 0 {
			detail += " Other saved Ethernet profiles exist, but none safely match this adapter."
		}
		check := model.DiagnosticCheck{
			ID:     "ethernet-profile-missing",
			Status: model.DiagnosticProblem,
			Title:  "No persistent Ethernet profile",
			Detail: detail,
		}
		if carrierUp {
			check.Repair = &model.RepairAction{
				Kind:          model.RepairCreateDHCPProfile,
				Label:         "Create automatic Ethernet profile",
				Description:   "Create a new persistent DHCP/autoconnect profile for this adapter. Existing saved profiles are not modified.",
				DevicePath:    device.ObjectPath,
				InterfaceName: device.Interface,
				Before:        "no matching profile",
				After:         "persistent automatic IPv4/IPv6 profile",
			}
		}
		checks = append(checks, withBase(base, check))
	}

	if macMismatch != nil {
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "ethernet-mac-mismatch",
			Status: model.DiagnosticProblem,
			Title:  "Saved profile is bound to a different hardware address",
			Detail: fmt.Sprintf("%q expects %s, but this adapter is %s. nm-hsp will not rewrite the MAC binding automatically.", macMismatch.ID, macMismatch.WiredMACAddress, device.HardwareAddress),
		}))
	}

	if device.ActiveConnection != nil {
		active := device.ActiveConnection
		switch {
		case len(device.IPv4.Addresses) > 0:
			checks = append(checks, withBase(base, model.DiagnosticCheck{
				ID:     "ethernet-ipv4-address",
				Status: model.DiagnosticOK,
				Title:  "IPv4 address is active",
				Detail: "Address: " + formatAddress(device.IPv4.Addresses[0]),
			}))
		case active.IPv4Method == "auto":
			check := model.DiagnosticCheck{
				ID:     "ethernet-dhcp-no-address",
				Status: model.DiagnosticProblem,
				Title:  "DHCP did not provide an IPv4 address",
				Detail: "The automatic IPv4 profile is active, but no IPv4 address is currently applied.",
			}
			if carrierUp {
				check.Repair = &model.RepairAction{
					Kind:          model.RepairActivateProfile,
					Label:         "Retry this connection",
					Description:   "Reactivate the same saved profile to retry NetworkManager DHCP. No profile settings are changed.",
					DevicePath:    device.ObjectPath,
					InterfaceName: device.Interface,
					ProfilePath:   active.ObjectPath,
					ProfileID:     active.ID,
					Before:        "active without IPv4 address",
					After:         "connection reactivated",
				}
			}
			checks = append(checks, withBase(base, check))
		case active.IPv4Method == "disabled":
			checks = append(checks, withBase(base, model.DiagnosticCheck{
				ID:     "ethernet-ipv4-disabled",
				Status: model.DiagnosticInfo,
				Title:  "IPv4 is disabled in the active profile",
				Detail: "This is a saved profile choice; nm-hsp will not override it during diagnostics.",
			}))
		default:
			checks = append(checks, withBase(base, model.DiagnosticCheck{
				ID:     "ethernet-ipv4-missing",
				Status: model.DiagnosticProblem,
				Title:  "Active profile has no IPv4 address",
				Detail: "The connection is active, but NetworkManager reports no effective IPv4 address.",
			}))
		}

		if len(device.IPv4.Addresses) > 0 {
			if device.IPv4.Gateway == "" {
				detail := "An IPv4 address is active, but no default IPv4 gateway is reported."
				if active.IPv4Method == "auto" {
					detail = "DHCP provided an IPv4 address, but NetworkManager reports no default IPv4 gateway."
				}
				checks = append(checks, withBase(base, model.DiagnosticCheck{
					ID:     "ethernet-no-gateway",
					Status: model.DiagnosticWarning,
					Title:  "No default IPv4 gateway",
					Detail: detail + " nm-hsp will not guess a gateway address.",
				}))
			} else {
				checks = append(checks, withBase(base, model.DiagnosticCheck{
					ID:     "ethernet-gateway-present",
					Status: model.DiagnosticOK,
					Title:  "Default gateway is configured",
					Detail: "Gateway: " + device.IPv4.Gateway,
				}))
			}
		}
	}

	return checks
}

func analyzeWiFi(snapshot model.Snapshot, device model.Device) []model.DiagnosticCheck {
	base := checkBase(device)
	checks := make([]model.DiagnosticCheck, 0, 4)

	if !snapshot.WirelessEnabled {
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "wifi-radio-disabled",
			Status: model.DiagnosticProblem,
			Title:  "Wi-Fi radio is turned off",
			Detail: "NetworkManager has Wi-Fi disabled.",
			Repair: &model.RepairAction{
				Kind:          model.RepairEnableWiFi,
				Label:         "Turn Wi-Fi on",
				Description:   "Enable the NetworkManager Wi-Fi radio.",
				DevicePath:    device.ObjectPath,
				InterfaceName: device.Interface,
				Before:        "Wi-Fi off",
				After:         "Wi-Fi on",
			},
		}))
	}
	if !device.Managed {
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "wifi-unmanaged",
			Status: model.DiagnosticProblem,
			Title:  "Wi-Fi adapter is not managed",
			Detail: "NetworkManager is not managing this Wi-Fi adapter. nm-hsp will not change management policy automatically.",
		}))
		return checks
	}

	switch device.State {
	case 20:
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "wifi-unavailable",
			Status: model.DiagnosticProblem,
			Title:  "Wi-Fi adapter is unavailable",
			Detail: "The adapter exists, but NetworkManager cannot currently use it. Check radio state, hardware/firmware, and platform restrictions.",
		}))
	case 120:
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "wifi-activation-failed",
			Status: model.DiagnosticProblem,
			Title:  "Wi-Fi activation failed",
			Detail: "NetworkManager reports a failed Wi-Fi activation. Open the Wi-Fi manager to review the saved profile or reconnect.",
		}))
	case 100:
		detail := "Wi-Fi is connected."
		if device.Wireless != nil && device.Wireless.SSID != "" {
			detail = fmt.Sprintf("Connected to %q.", device.Wireless.SSID)
		}
		checks = append(checks, withBase(base, model.DiagnosticCheck{
			ID:     "wifi-connected",
			Status: model.DiagnosticOK,
			Title:  "Wi-Fi is connected",
			Detail: detail,
		}))
	default:
		if snapshot.WirelessEnabled {
			checks = append(checks, withBase(base, model.DiagnosticCheck{
				ID:     "wifi-not-connected",
				Status: model.DiagnosticInfo,
				Title:  "Wi-Fi is not connected",
				Detail: "The adapter is available but has no active Wi-Fi connection.",
			}))
		}
	}
	return checks
}

func analyzeDNS(snapshot model.Snapshot, probe model.DiagnosticProbe) []model.DiagnosticCheck {
	hasAddress := false
	hasDNS := false
	for _, device := range snapshot.Devices {
		if device.Kind != model.DeviceKindEthernet && device.Kind != model.DeviceKindWiFi {
			continue
		}
		if len(device.IPv4.Addresses) > 0 || len(device.IPv6.Addresses) > 0 {
			hasAddress = true
		}
		if len(device.IPv4.DNS) > 0 || len(device.IPv6.DNS) > 0 {
			hasDNS = true
		}
	}
	if !hasAddress {
		return nil
	}
	if !hasDNS {
		return []model.DiagnosticCheck{{
			ID:     "dns-not-configured",
			Status: model.DiagnosticWarning,
			Title:  "No DNS servers are active",
			Detail: "NetworkManager reports an IP address but no DNS servers. nm-hsp will not guess DNS addresses.",
		}}
	}
	if !probe.DNSAttempted {
		return []model.DiagnosticCheck{{
			ID:     "dns-configured",
			Status: model.DiagnosticOK,
			Title:  "DNS servers are configured",
			Detail: "NetworkManager reports at least one DNS server.",
		}}
	}
	if probe.DNSWorking {
		return []model.DiagnosticCheck{{
			ID:     "dns-working",
			Status: model.DiagnosticOK,
			Title:  "DNS lookup works",
			Detail: "A test lookup completed successfully.",
		}}
	}
	detail := "DNS servers are configured, but a DNS test lookup failed."
	return []model.DiagnosticCheck{{
		ID:     "dns-lookup-failed",
		Status: model.DiagnosticProblem,
		Title:  "DNS lookup failed",
		Detail: detail,
	}}
}

func analyzeConnectivity(snapshot model.Snapshot, probe model.DiagnosticProbe) model.DiagnosticCheck {
	switch model.ConnectivityName(snapshot.Connectivity) {
	case "full":
		return model.DiagnosticCheck{
			ID:     "internet-full",
			Status: model.DiagnosticOK,
			Title:  "Internet connectivity is available",
			Detail: "NetworkManager reports full connectivity.",
		}
	case "portal":
		return model.DiagnosticCheck{
			ID:     "internet-portal",
			Status: model.DiagnosticWarning,
			Title:  "Captive portal detected",
			Detail: "NetworkManager reports portal connectivity. A sign-in page may be required.",
		}
	case "limited":
		detail := "NetworkManager reports limited connectivity."
		if probe.DNSAttempted && probe.DNSWorking {
			detail = "DNS lookup works, but NetworkManager still reports limited Internet connectivity."
		}
		return model.DiagnosticCheck{
			ID:     "internet-limited",
			Status: model.DiagnosticWarning,
			Title:  "Internet connectivity is limited",
			Detail: detail,
		}
	case "none":
		detail := "NetworkManager reports no Internet connectivity."
		if probe.DNSAttempted && probe.DNSWorking {
			detail = "DNS lookup works, but NetworkManager reports no Internet connectivity."
		}
		return model.DiagnosticCheck{
			ID:     "internet-none",
			Status: model.DiagnosticProblem,
			Title:  "Internet connectivity is unavailable",
			Detail: detail,
		}
	default:
		return model.DiagnosticCheck{
			ID:     "internet-unknown",
			Status: model.DiagnosticInfo,
			Title:  "Internet connectivity is unknown",
			Detail: "NetworkManager did not report a definitive connectivity state.",
		}
	}
}

func autoconnectCheck(device model.Device, profile model.ConnectionProfile) model.DiagnosticCheck {
	return withBase(checkBase(device), model.DiagnosticCheck{
		ID:     "profile-autoconnect-disabled",
		Status: model.DiagnosticWarning,
		Title:  "Automatic reconnect is disabled",
		Detail: fmt.Sprintf("%q will not automatically reconnect after boot or link recovery.", profile.ID),
		Repair: &model.RepairAction{
			Kind:          model.RepairEnableAutoconnect,
			Label:         "Enable autoconnect",
			Description:   "Change only the saved autoconnect flag. IP settings and other profile options stay unchanged.",
			DevicePath:    device.ObjectPath,
			InterfaceName: device.Interface,
			ProfilePath:   profile.ObjectPath,
			ProfileID:     profile.ID,
			Before:        "autoconnect off",
			After:         "autoconnect on",
		},
	})
}

func ethernetProfiles(profiles []model.ConnectionProfile) []model.ConnectionProfile {
	result := make([]model.ConnectionProfile, 0)
	for _, profile := range profiles {
		if profile.Type == "802-3-ethernet" {
			result = append(result, profile)
		}
	}
	return result
}

func bestEthernetProfile(device model.Device, profiles []model.ConnectionProfile) *model.ConnectionProfile {
	available := make(map[string]struct{}, len(device.AvailableProfileUUIDs))
	for _, uuid := range device.AvailableProfileUUIDs {
		available[uuid] = struct{}{}
	}

	type scored struct {
		profile model.ConnectionProfile
		score   int
	}
	var candidates []scored
	for _, profile := range profiles {
		if profile.WiredMACAddress != "" && !sameMAC(profile.WiredMACAddress, device.HardwareAddress) {
			continue
		}
		score := 0
		if _, ok := available[profile.UUID]; ok {
			score += 100
		}
		if profile.InterfaceName != "" && profile.InterfaceName == device.Interface {
			score += 80
		}
		if sameMAC(profile.WiredMACAddress, device.HardwareAddress) {
			score += 90
		}
		if score == 0 {
			continue
		}
		if profile.Autoconnect {
			score += 10
		}
		candidates = append(candidates, scored{profile: profile, score: score})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].profile.AutoconnectPriority != candidates[j].profile.AutoconnectPriority {
			return candidates[i].profile.AutoconnectPriority > candidates[j].profile.AutoconnectPriority
		}
		return candidates[i].profile.ID < candidates[j].profile.ID
	})
	profile := candidates[0].profile
	return &profile
}

func ethernetMACMismatchProfile(device model.Device, profiles []model.ConnectionProfile) *model.ConnectionProfile {
	for _, profile := range profiles {
		if profile.WiredMACAddress == "" || sameMAC(profile.WiredMACAddress, device.HardwareAddress) {
			continue
		}
		if profile.InterfaceName == device.Interface || profile.InterfaceName == "" {
			copy := profile
			return &copy
		}
	}
	return nil
}

func unmatchedEthernetProfiles(device model.Device, profiles []model.ConnectionProfile) []model.ConnectionProfile {
	var result []model.ConnectionProfile
	for _, profile := range profiles {
		if profile.InterfaceName != "" && profile.InterfaceName != device.Interface {
			result = append(result, profile)
			continue
		}
		if profile.WiredMACAddress != "" && !sameMAC(profile.WiredMACAddress, device.HardwareAddress) {
			result = append(result, profile)
		}
	}
	return result
}

func sameMAC(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	normalize := func(value string) string {
		return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), "-", ":"))
	}
	return normalize(left) == normalize(right)
}

func checkBase(device model.Device) model.DiagnosticCheck {
	return model.DiagnosticCheck{
		Interface:  device.Interface,
		DevicePath: device.ObjectPath,
	}
}

func withBase(base, check model.DiagnosticCheck) model.DiagnosticCheck {
	check.Interface = base.Interface
	check.DevicePath = base.DevicePath
	return check
}

func formatAddress(address model.IPAddress) string {
	return fmt.Sprintf("%s/%d", address.Address, address.Prefix)
}
