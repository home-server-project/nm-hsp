// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/home-server-project/nm-hsp/internal/model"
)

const (
	minEthernetMTU = 68
	minIPv6MTU     = 1280
	maxEthernetMTU = 65535
)

// ParseAddresses parses a comma-separated list of address/prefix values.
func ParseAddresses(input string, ipv6 bool) ([]model.IPAddress, error) {
	parts := splitList(input)
	if len(parts) == 0 {
		return nil, nil
	}

	addresses := make([]model.IPAddress, 0, len(parts))
	for _, value := range parts {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("%q must be an IP address with prefix length", value)
		}
		if prefix.Addr().Is6() != ipv6 {
			return nil, fmt.Errorf("%q is the wrong IP family", value)
		}
		if prefix.Addr().IsUnspecified() || prefix.Addr().IsMulticast() {
			return nil, fmt.Errorf("%q is not a usable interface address", value)
		}
		addresses = append(addresses, model.IPAddress{
			Address: prefix.Addr().String(),
			Prefix:  uint32(prefix.Bits()),
		})
	}
	return addresses, nil
}

// ParseGateway parses an optional gateway address.
func ParseGateway(input string, ipv6 bool) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}

	address, err := netip.ParseAddr(value)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid gateway address", value)
	}
	if address.Is6() != ipv6 {
		return "", fmt.Errorf("%q is the wrong IP family for this gateway", value)
	}
	if address.IsUnspecified() || address.IsMulticast() {
		return "", fmt.Errorf("%q is not a usable gateway address", value)
	}
	return address.String(), nil
}

// ParseDNS parses a comma-separated list of plain DNS server IP addresses.
func ParseDNS(input string, ipv6 bool) ([]string, error) {
	parts := splitList(input)
	if len(parts) == 0 {
		return nil, nil
	}

	servers := make([]string, 0, len(parts))
	for _, value := range parts {
		address, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("%q is not a valid DNS server address", value)
		}
		if address.Is6() != ipv6 {
			return nil, fmt.Errorf("%q is the wrong IP family for DNS", value)
		}
		if address.IsUnspecified() || address.IsMulticast() {
			return nil, fmt.Errorf("%q is not a usable DNS server address", value)
		}
		servers = append(servers, address.String())
	}
	return servers, nil
}

// ParseMTU parses a user-entered Ethernet MTU. Blank and zero mean automatic.
func ParseMTU(input string, ipv6Enabled bool) (uint32, error) {
	value := strings.TrimSpace(input)
	if value == "" || value == "0" {
		return 0, nil
	}

	mtu, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid MTU", value)
	}
	if mtu < minEthernetMTU || mtu > maxEthernetMTU {
		return 0, fmt.Errorf("MTU must be 0 (automatic) or between %d and %d", minEthernetMTU, maxEthernetMTU)
	}
	if ipv6Enabled && mtu < minIPv6MTU {
		return 0, fmt.Errorf("MTU must be at least %d while IPv6 is enabled", minIPv6MTU)
	}
	return uint32(mtu), nil
}

// ValidateIPConfig validates the normalized persistent IP configuration.
func ValidateIPConfig(config model.IPProfileConfig, ipv6 bool) error {
	switch config.Method {
	case model.IPMethodAuto:
		if len(config.Addresses) != 0 || config.Gateway != "" {
			return fmt.Errorf("%s automatic mode cannot contain manual addresses or gateway", familyName(ipv6))
		}
	case model.IPMethodManual:
		if len(config.Addresses) == 0 {
			return fmt.Errorf("%s manual mode requires at least one address", familyName(ipv6))
		}
	case model.IPMethodDisabled:
		if len(config.Addresses) != 0 || config.Gateway != "" || len(config.DNS) != 0 {
			return fmt.Errorf("%s disabled mode cannot contain addresses, gateway, or DNS", familyName(ipv6))
		}
	default:
		return fmt.Errorf("unsupported %s method %q", familyName(ipv6), config.Method)
	}

	for _, entry := range config.Addresses {
		prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", entry.Address, entry.Prefix))
		if err != nil || prefix.Addr().Is6() != ipv6 {
			return fmt.Errorf("invalid %s address %s/%d", familyName(ipv6), entry.Address, entry.Prefix)
		}
	}
	if _, err := ParseGateway(config.Gateway, ipv6); err != nil {
		return err
	}
	if _, err := ParseDNS(strings.Join(config.DNS, ","), ipv6); err != nil {
		return err
	}
	return nil
}

// ValidateEthernetProfile validates all supported editable Ethernet fields.
func ValidateEthernetProfile(profile model.EthernetProfile) error {
	if strings.TrimSpace(profile.InterfaceName) == "" {
		return fmt.Errorf("Ethernet interface name is required")
	}
	if profile.MTU != 0 {
		if profile.MTU < minEthernetMTU || profile.MTU > maxEthernetMTU {
			return fmt.Errorf("MTU must be 0 (automatic) or between %d and %d", minEthernetMTU, maxEthernetMTU)
		}
		if profile.IPv6.Method != model.IPMethodDisabled && profile.MTU < minIPv6MTU {
			return fmt.Errorf("MTU must be at least %d while IPv6 is enabled", minIPv6MTU)
		}
	}
	if err := ValidateIPConfig(profile.IPv4, false); err != nil {
		return err
	}
	if err := ValidateIPConfig(profile.IPv6, true); err != nil {
		return err
	}
	return nil
}

func splitList(input string) []string {
	raw := strings.Split(input, ",")
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if value := strings.TrimSpace(item); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func familyName(ipv6 bool) string {
	if ipv6 {
		return "IPv6"
	}
	return "IPv4"
}
