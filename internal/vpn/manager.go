// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/home-server-project/nm-hsp/internal/model"
)

// Manager collects the supported private-access providers in a stable order.
// Step 1 is intentionally read-only: it detects provider presence, systemd
// lifecycle state, and provider status where a supported local API is present.
type Manager struct {
	providers []providerSpec
	services  serviceStateReader
	status    providerStatusReader
	installed executableDetector
}

type providerSpec struct {
	id         model.VPNProviderID
	name       string
	executable string
	service    string
}

type serviceState struct {
	enabled bool
	active  string
}

type providerStatus struct {
	state     string
	connected bool
	addresses []string
}

type serviceStateReader interface {
	UnitState(context.Context, string) (serviceState, error)
}

type providerStatusReader interface {
	Status(context.Context, model.VPNProviderID) (providerStatus, bool, error)
}

type executableDetector interface {
	Installed(string) bool
}

// NewManager builds the host provider manager used by nm-hsp.
func NewManager() *Manager {
	return newManager(systemdReader{}, localAPIReader{}, pathExecutableDetector{})
}

func newManager(services serviceStateReader, status providerStatusReader, installed executableDetector) *Manager {
	return &Manager{
		providers: []providerSpec{
			{id: model.VPNProviderTailscale, name: "Tailscale", executable: "tailscale", service: "tailscaled.service"},
			{id: model.VPNProviderNetBird, name: "NetBird", executable: "netbird", service: "netbird.service"},
		},
		services:  services,
		status:    status,
		installed: installed,
	}
}

// Snapshot returns every supported provider, including providers that are not
// installed. Provider-specific failures are contained in StatusError and never
// prevent the remaining provider states from being collected.
func (m *Manager) Snapshot(ctx context.Context) []model.VPNProviderState {
	states := make([]model.VPNProviderState, 0, len(m.providers))
	for _, provider := range m.providers {
		states = append(states, m.providerState(ctx, provider))
	}
	return states
}

func (m *Manager) providerState(ctx context.Context, provider providerSpec) model.VPNProviderState {
	state := model.VPNProviderState{
		ID:          provider.id,
		Name:        provider.name,
		ServiceUnit: provider.service,
	}

	if !m.installed.Installed(provider.executable) {
		return state
	}
	state.Installed = true

	service, err := m.services.UnitState(ctx, provider.service)
	if err != nil {
		state.ConnectionState = "unknown"
		state.StatusError = "system service state is unavailable"
		return state
	}
	state.ServiceEnabled = service.enabled
	state.ServiceState = service.active
	state.ServiceRunning = service.active == "active"

	if !state.ServiceRunning {
		state.ConnectionState = "inactive"
		return state
	}

	providerState, available, err := m.status.Status(ctx, provider.id)
	if errors.Is(err, errStatusUnavailable) || !available {
		state.ConnectionState = "unavailable"
		return state
	}
	if err != nil {
		state.ConnectionState = "unknown"
		state.StatusError = "provider status is unavailable"
		return state
	}

	state.ConnectionState = providerState.state
	state.Connected = providerState.connected
	state.Addresses = providerState.addresses
	return state
}

type pathExecutableDetector struct{}

func (pathExecutableDetector) Installed(name string) bool {
	if name == "" || strings.ContainsRune(name, filepath.Separator) {
		return false
	}

	paths := filepath.SplitList(os.Getenv("PATH"))
	paths = append(paths, "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin")
	seen := make(map[string]struct{}, len(paths))
	for _, dir := range paths {
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}

		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode().Perm()&0o111 != 0 {
			return true
		}
	}
	return false
}
