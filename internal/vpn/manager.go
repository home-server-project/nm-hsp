// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"encoding/json"
	"net/netip"
	"os/exec"
	"strings"

	"github.com/home-server-project/nm-hsp/internal/model"
)

// Provider exposes read-only state for one supported private-access provider.
// Mutating lifecycle and authentication actions are deliberately outside this
// Step 1 contract and will be added separately.
type Provider interface {
	ID() model.VPNProviderID
	Name() string
	ServiceUnit() string
	Status(context.Context) model.VPNProviderState
}

// Manager collects the supported private-access providers in a stable order.
type Manager struct {
	providers []Provider
}

// NewManager builds the host provider manager used by nm-hsp.
func NewManager() *Manager {
	return newManager(execRunner{})
}

// Snapshot returns every supported provider, including providers that are not
// installed. A provider-specific failure is contained in that provider's
// StatusError and never prevents the remaining provider states from being read.
func (m *Manager) Snapshot(ctx context.Context) []model.VPNProviderState {
	states := make([]model.VPNProviderState, 0, len(m.providers))
	for _, provider := range m.providers {
		states = append(states, provider.Status(ctx))
	}
	return states
}

type commandRunner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type commandProvider struct {
	runner     commandRunner
	id         model.VPNProviderID
	name       string
	executable string
	service    string
	statusArgs []string
	parse      func([]byte) (connectionState string, connected bool, addresses []string, err error)
}

func newManager(runner commandRunner) *Manager {
	return &Manager{providers: []Provider{
		&commandProvider{
			runner:     runner,
			id:         model.VPNProviderTailscale,
			name:       "Tailscale",
			executable: "tailscale",
			service:    "tailscaled.service",
			statusArgs: []string{"status", "--json"},
			parse:      parseTailscaleStatus,
		},
		&commandProvider{
			runner:     runner,
			id:         model.VPNProviderNetBird,
			name:       "NetBird",
			executable: "netbird",
			service:    "netbird.service",
			statusArgs: []string{"status", "--json"},
			parse:      parseNetBirdStatus,
		},
	}}
}

func (p *commandProvider) ID() model.VPNProviderID { return p.id }
func (p *commandProvider) Name() string            { return p.name }
func (p *commandProvider) ServiceUnit() string     { return p.service }

func (p *commandProvider) Status(ctx context.Context) model.VPNProviderState {
	state := model.VPNProviderState{
		ID:          p.id,
		Name:        p.name,
		ServiceUnit: p.service,
	}

	if _, err := p.runner.LookPath(p.executable); err != nil {
		return state
	}
	state.Installed = true

	enabledState, enabledErr := systemctlState(ctx, p.runner, "is-enabled", p.service)
	state.ServiceEnabled = enabledState == "enabled"
	if enabledErr != nil {
		state.StatusError = "system service enablement is unavailable"
	}

	activeState, activeErr := systemctlState(ctx, p.runner, "is-active", p.service)
	state.ServiceState = activeState
	state.ServiceRunning = activeState == "active"
	if activeErr != nil {
		state.StatusError = joinStatusError(state.StatusError, "system service state is unavailable")
	}

	if !state.ServiceRunning {
		state.ConnectionState = "inactive"
		return state
	}

	output, commandErr := p.runner.Run(ctx, p.executable, p.statusArgs...)
	connectionState, connected, addresses, parseErr := p.parse(output)
	if parseErr != nil {
		state.ConnectionState = "unknown"
		state.StatusError = joinStatusError(state.StatusError, "provider status is unavailable")
		return state
	}

	state.ConnectionState = connectionState
	state.Connected = connected
	state.Addresses = addresses
	if commandErr != nil && len(output) == 0 {
		state.StatusError = joinStatusError(state.StatusError, "provider status command failed")
	}
	return state
}

func systemctlState(ctx context.Context, runner commandRunner, action, service string) (string, error) {
	output, err := runner.Run(ctx, "systemctl", action, service)
	state := strings.TrimSpace(string(output))
	if state != "" {
		return state, nil
	}
	return state, err
}

func joinStatusError(current, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

type tailscaleStatus struct {
	BackendState string   `json:"BackendState"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

func parseTailscaleStatus(data []byte) (string, bool, []string, error) {
	var status tailscaleStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return "", false, nil, err
	}
	state := strings.TrimSpace(status.BackendState)
	if state == "" {
		state = "unknown"
	}
	return state, state == "Running", normalizeAddresses(status.TailscaleIPs), nil
}

type netBirdStatus struct {
	DaemonStatus string `json:"daemonStatus"`
	Management   struct {
		Connected bool `json:"connected"`
	} `json:"management"`
	IP   string `json:"netbirdIp"`
	IPv6 string `json:"netbirdIpv6"`
}

func parseNetBirdStatus(data []byte) (string, bool, []string, error) {
	var status netBirdStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return "", false, nil, err
	}

	state := strings.TrimSpace(status.DaemonStatus)
	if state == "" {
		if status.Management.Connected {
			state = "Connected"
		} else {
			state = "Disconnected"
		}
	}
	connected := strings.EqualFold(state, "Connected") && status.Management.Connected
	return state, connected, normalizeAddresses([]string{status.IP, status.IPv6}), nil
}

func normalizeAddresses(values []string) []string {
	addresses := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		address := ""
		if prefix, err := netip.ParsePrefix(value); err == nil {
			address = prefix.Addr().String()
		} else if parsed, err := netip.ParseAddr(value); err == nil {
			address = parsed.String()
		}
		if address == "" {
			continue
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses
}
