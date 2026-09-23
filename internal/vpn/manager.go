// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/home-server-project/nm-hsp/internal/model"
)

// Manager owns the small, provider-neutral private-access lifecycle surface.
type Manager struct {
	providers []providerSpec
	services  serviceManager
	backend   providerBackend
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
	authURL   string
}

type providerActionResult struct {
	message      string
	authURL      string
	userCode     string
	awaitingAuth bool
}

type serviceManager interface {
	UnitState(context.Context, string) (serviceState, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
}

type providerBackend interface {
	Status(context.Context, model.VPNProviderID) (providerStatus, bool, error)
	Connect(context.Context, model.VPNProviderID) (providerActionResult, error)
	Disconnect(context.Context, model.VPNProviderID) error
	WaitAuthentication(context.Context, model.VPNProviderID, string) (providerActionResult, error)
}

type executableDetector interface {
	Installed(string) bool
}

// NewManager builds the host provider manager used by nm-hsp.
func NewManager() *Manager {
	return newManager(systemdReader{}, newLocalProviderBackend(), pathExecutableDetector{})
}

func newManager(services serviceManager, backend providerBackend, installed executableDetector) *Manager {
	return &Manager{
		providers: []providerSpec{
			{id: model.VPNProviderTailscale, name: "Tailscale", executable: "tailscale", service: "tailscaled.service"},
			{id: model.VPNProviderNetBird, name: "NetBird", executable: "netbird", service: "netbird.service"},
		},
		services:  services,
		backend:   backend,
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

// Action performs one explicit local lifecycle operation. It never installs a
// provider package and never changes provider-side policy or account settings.
func (m *Manager) Action(ctx context.Context, id model.VPNProviderID, action model.VPNAction) (model.VPNActionResult, error) {
	provider, ok := m.provider(id)
	if !ok {
		return model.VPNActionResult{}, fmt.Errorf("unsupported private access provider %q", id)
	}
	if !m.installed.Installed(provider.executable) {
		return model.VPNActionResult{}, fmt.Errorf("%s is not installed", provider.name)
	}

	switch action {
	case model.VPNActionActivate:
		if err := m.services.Start(ctx, provider.service); err != nil {
			return model.VPNActionResult{}, fmt.Errorf("activate %s service: %w", provider.name, err)
		}
		return m.connect(ctx, provider)

	case model.VPNActionConnect:
		state, err := m.services.UnitState(ctx, provider.service)
		if err != nil {
			return model.VPNActionResult{}, fmt.Errorf("read %s service state: %w", provider.name, err)
		}
		if state.active != "active" {
			return model.VPNActionResult{}, fmt.Errorf("%s service is not running; activate it first", provider.name)
		}
		return m.connect(ctx, provider)

	case model.VPNActionReconnect:
		state, err := m.services.UnitState(ctx, provider.service)
		if err != nil {
			return model.VPNActionResult{}, fmt.Errorf("read %s service state: %w", provider.name, err)
		}
		if state.active != "active" {
			return model.VPNActionResult{}, fmt.Errorf("%s service is not running; activate it first", provider.name)
		}
		_ = m.backend.Disconnect(ctx, provider.id)
		return m.connect(ctx, provider)

	case model.VPNActionDisconnect:
		if err := m.backend.Disconnect(ctx, provider.id); err != nil {
			return model.VPNActionResult{}, fmt.Errorf("disconnect %s: %w", provider.name, err)
		}
		return model.VPNActionResult{Message: provider.name + " disconnected. Service remains running."}, nil

	case model.VPNActionDeactivate:
		if err := m.services.Stop(ctx, provider.service); err != nil {
			return model.VPNActionResult{}, fmt.Errorf("deactivate %s service: %w", provider.name, err)
		}
		return model.VPNActionResult{Message: provider.name + " service stopped."}, nil

	default:
		return model.VPNActionResult{}, fmt.Errorf("unsupported private access action %q", action)
	}
}

// WaitAuthentication completes a previously-started browser/device-code login.
// The caller supplies only the short-lived provider handle returned by Action.
func (m *Manager) WaitAuthentication(ctx context.Context, id model.VPNProviderID, userCode string) (model.VPNActionResult, error) {
	provider, ok := m.provider(id)
	if !ok {
		return model.VPNActionResult{}, fmt.Errorf("unsupported private access provider %q", id)
	}
	if !m.installed.Installed(provider.executable) {
		return model.VPNActionResult{}, fmt.Errorf("%s is not installed", provider.name)
	}

	result, err := m.backend.WaitAuthentication(ctx, id, userCode)
	if err != nil {
		return model.VPNActionResult{}, err
	}
	return exportActionResult(result), nil
}

func (m *Manager) connect(ctx context.Context, provider providerSpec) (model.VPNActionResult, error) {
	result, err := m.backend.Connect(ctx, provider.id)
	if err != nil {
		return model.VPNActionResult{}, fmt.Errorf("connect %s: %w", provider.name, err)
	}
	if result.message == "" {
		result.message = provider.name + " connection requested."
	}
	return exportActionResult(result), nil
}

func exportActionResult(result providerActionResult) model.VPNActionResult {
	return model.VPNActionResult{
		Message:      result.message,
		AuthURL:      result.authURL,
		UserCode:     result.userCode,
		AwaitingAuth: result.awaitingAuth,
	}
}

func (m *Manager) provider(id model.VPNProviderID) (providerSpec, bool) {
	for _, provider := range m.providers {
		if provider.id == id {
			return provider, true
		}
	}
	return providerSpec{}, false
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

	providerState, available, err := m.backend.Status(ctx, provider.id)
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
	if state.Connected {
		state.Addresses = providerState.addresses
	}
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
