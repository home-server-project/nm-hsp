// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

type fakeServices struct {
	states       map[string]serviceState
	errs         map[string]error
	enabledStart []string
	stopped      []string
}

func (f *fakeServices) UnitState(_ context.Context, unit string) (serviceState, error) {
	if err := f.errs[unit]; err != nil {
		return serviceState{}, err
	}
	return f.states[unit], nil
}

func (f *fakeServices) EnableAndStart(_ context.Context, unit string) error {
	f.enabledStart = append(f.enabledStart, unit)
	if f.states == nil {
		f.states = map[string]serviceState{}
	}
	f.states[unit] = serviceState{enabled: true, active: "active"}
	return nil
}

func (f *fakeServices) StopAndDisable(_ context.Context, unit string) error {
	f.stopped = append(f.stopped, unit)
	if f.states == nil {
		f.states = map[string]serviceState{}
	}
	f.states[unit] = serviceState{enabled: false, active: "inactive"}
	return nil
}

type fakeProviderBackend struct {
	states      map[model.VPNProviderID]providerStatus
	available   map[model.VPNProviderID]bool
	errs        map[model.VPNProviderID]error
	connect     map[model.VPNProviderID]providerActionResult
	connectErr  map[model.VPNProviderID]error
	disconnects []model.VPNProviderID
	wait        map[model.VPNProviderID]providerActionResult
}

func (f *fakeProviderBackend) Status(_ context.Context, id model.VPNProviderID) (providerStatus, bool, error) {
	return f.states[id], f.available[id], f.errs[id]
}

func (f *fakeProviderBackend) Connect(_ context.Context, id model.VPNProviderID) (providerActionResult, error) {
	return f.connect[id], f.connectErr[id]
}

func (f *fakeProviderBackend) Disconnect(_ context.Context, id model.VPNProviderID) error {
	f.disconnects = append(f.disconnects, id)
	return nil
}

func (f *fakeProviderBackend) WaitAuthentication(_ context.Context, id model.VPNProviderID, _ string) (providerActionResult, error) {
	return f.wait[id], nil
}

type fakeInstalled map[string]bool

func (f fakeInstalled) Installed(name string) bool { return f[name] }

func TestSnapshotAlwaysListsSupportedProviders(t *testing.T) {
	manager := newManager(&fakeServices{}, &fakeProviderBackend{}, fakeInstalled{})
	states := manager.Snapshot(context.Background())

	if len(states) != 2 {
		t.Fatalf("provider count = %d, want 2", len(states))
	}
	if states[0].ID != model.VPNProviderTailscale || states[0].Installed {
		t.Fatalf("tailscale state = %#v", states[0])
	}
	if states[1].ID != model.VPNProviderNetBird || states[1].Installed {
		t.Fatalf("netbird state = %#v", states[1])
	}
}

func TestTailscaleConnectedState(t *testing.T) {
	manager := newManager(
		&fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: true, active: "active"}}},
		&fakeProviderBackend{
			states: map[model.VPNProviderID]providerStatus{
				model.VPNProviderTailscale: {state: "Running", connected: true, addresses: []string{"100.64.0.10"}},
			},
			available: map[model.VPNProviderID]bool{model.VPNProviderTailscale: true},
		},
		fakeInstalled{"tailscale": true},
	)

	state := manager.Snapshot(context.Background())[0]
	if !state.Installed || !state.ServiceEnabled || !state.ServiceRunning || !state.Connected {
		t.Fatalf("tailscale state = %#v", state)
	}
	if state.ConnectionState != "Running" || !reflect.DeepEqual(state.Addresses, []string{"100.64.0.10"}) {
		t.Fatalf("tailscale state = %#v", state)
	}
}

func TestInactiveServiceNeedsNoProviderStatus(t *testing.T) {
	manager := newManager(
		&fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: false, active: "inactive"}}},
		&fakeProviderBackend{errs: map[model.VPNProviderID]error{model.VPNProviderTailscale: errors.New("must not be observed")}},
		fakeInstalled{"tailscale": true},
	)

	state := manager.Snapshot(context.Background())[0]
	if state.ServiceRunning || state.ConnectionState != "inactive" || state.StatusError != "" {
		t.Fatalf("tailscale state = %#v", state)
	}
}

func TestActivateStartsServiceAndReturnsAuthenticationHandoff(t *testing.T) {
	services := &fakeServices{states: map[string]serviceState{}}
	backend := &fakeProviderBackend{
		connect: map[model.VPNProviderID]providerActionResult{
			model.VPNProviderTailscale: {
				message:      "login",
				authURL:      "https://login.example/",
				awaitingAuth: true,
			},
		},
	}
	manager := newManager(services, backend, fakeInstalled{"tailscale": true})

	result, err := manager.Action(context.Background(), model.VPNProviderTailscale, model.VPNActionActivate)
	if err != nil {
		t.Fatal(err)
	}
	if len(services.enabledStart) != 1 || services.enabledStart[0] != "tailscaled.service" {
		t.Fatalf("started services = %#v", services.enabledStart)
	}
	if !result.AwaitingAuth || result.AuthURL == "" {
		t.Fatalf("action result = %#v", result)
	}
}

func TestDisconnectKeepsServiceEnabled(t *testing.T) {
	services := &fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: true, active: "active"}}}
	backend := &fakeProviderBackend{}
	manager := newManager(services, backend, fakeInstalled{"tailscale": true})

	result, err := manager.Action(context.Background(), model.VPNProviderTailscale, model.VPNActionDisconnect)
	if err != nil {
		t.Fatal(err)
	}
	if len(backend.disconnects) != 1 {
		t.Fatalf("disconnects = %#v", backend.disconnects)
	}
	if len(services.stopped) != 0 {
		t.Fatal("disconnect must not stop or disable the service")
	}
	if result.Message == "" {
		t.Fatal("disconnect result should explain service remains enabled")
	}
}

func TestDeactivateStopsAndDisablesService(t *testing.T) {
	services := &fakeServices{}
	manager := newManager(services, &fakeProviderBackend{}, fakeInstalled{"netbird": true})

	_, err := manager.Action(context.Background(), model.VPNProviderNetBird, model.VPNActionDeactivate)
	if err != nil {
		t.Fatal(err)
	}
	if len(services.stopped) != 1 || services.stopped[0] != "netbird.service" {
		t.Fatalf("stopped services = %#v", services.stopped)
	}
}

func TestWaitAuthenticationDoesNotExposeHandleInSnapshot(t *testing.T) {
	backend := &fakeProviderBackend{
		wait: map[model.VPNProviderID]providerActionResult{
			model.VPNProviderNetBird: {message: "connected"},
		},
	}
	manager := newManager(&fakeServices{}, backend, fakeInstalled{"netbird": true})

	result, err := manager.WaitAuthentication(context.Background(), model.VPNProviderNetBird, "ABCD-EFGH")
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "connected" || result.AuthURL != "" || result.UserCode != "" || result.AwaitingAuth {
		t.Fatalf("wait result = %#v", result)
	}
}

func TestProviderStatusFailureIsContained(t *testing.T) {
	manager := newManager(
		&fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: true, active: "active"}}},
		&fakeProviderBackend{
			available: map[model.VPNProviderID]bool{model.VPNProviderTailscale: true},
			errs:      map[model.VPNProviderID]error{model.VPNProviderTailscale: errors.New("bad response")},
		},
		fakeInstalled{"tailscale": true},
	)

	states := manager.Snapshot(context.Background())
	if states[0].ConnectionState != "unknown" || states[0].StatusError == "" {
		t.Fatalf("tailscale failure state = %#v", states[0])
	}
	if states[1].ID != model.VPNProviderNetBird {
		t.Fatalf("remaining provider was not collected: %#v", states)
	}
}

func TestServiceFailureIsContained(t *testing.T) {
	manager := newManager(
		&fakeServices{errs: map[string]error{"tailscaled.service": errors.New("dbus unavailable")}},
		&fakeProviderBackend{},
		fakeInstalled{"tailscale": true},
	)

	state := manager.Snapshot(context.Background())[0]
	if state.ConnectionState != "unknown" || state.StatusError == "" {
		t.Fatalf("tailscale state = %#v", state)
	}
}

func TestNormalizeTailscaleStatus(t *testing.T) {
	state := normalizeTailscaleStatus(tailscaleStatus{
		BackendState: "Running",
		TailscaleIPs: []string{"100.64.0.10", "fd7a:115c:a1e0::10"},
	})
	if !state.connected || state.state != "Running" {
		t.Fatalf("tailscale provider status = %#v", state)
	}
	want := []string{"100.64.0.10", "fd7a:115c:a1e0::10"}
	if !reflect.DeepEqual(state.addresses, want) {
		t.Fatalf("addresses = %#v, want %#v", state.addresses, want)
	}
}

func TestNormalizeNetBirdRPCStatus(t *testing.T) {
	state := normalizeNetBirdRPCStatus(nbStatusResponse{
		Status: "Connected",
		FullStatus: &nbFullStatus{
			LocalPeerState: &nbLocalPeerState{IP: "100.119.62.6/16", IPv6: "fd00::6/64"},
		},
	})
	if !state.connected || state.state != "Connected" {
		t.Fatalf("netbird provider status = %#v", state)
	}
	want := []string{"100.119.62.6", "fd00::6"}
	if !reflect.DeepEqual(state.addresses, want) {
		t.Fatalf("addresses = %#v, want %#v", state.addresses, want)
	}
}
