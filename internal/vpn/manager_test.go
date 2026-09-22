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
	states map[string]serviceState
	errs   map[string]error
}

func (f fakeServices) UnitState(_ context.Context, unit string) (serviceState, error) {
	if err := f.errs[unit]; err != nil {
		return serviceState{}, err
	}
	return f.states[unit], nil
}

type fakeProviderStatus struct {
	states    map[model.VPNProviderID]providerStatus
	available map[model.VPNProviderID]bool
	errs      map[model.VPNProviderID]error
}

func (f fakeProviderStatus) Status(_ context.Context, id model.VPNProviderID) (providerStatus, bool, error) {
	return f.states[id], f.available[id], f.errs[id]
}

type fakeInstalled map[string]bool

func (f fakeInstalled) Installed(name string) bool { return f[name] }

func TestSnapshotAlwaysListsSupportedProviders(t *testing.T) {
	manager := newManager(fakeServices{}, fakeProviderStatus{}, fakeInstalled{})
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
		fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: true, active: "active"}}},
		fakeProviderStatus{
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
		fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: false, active: "inactive"}}},
		fakeProviderStatus{errs: map[model.VPNProviderID]error{model.VPNProviderTailscale: errors.New("must not be observed")}},
		fakeInstalled{"tailscale": true},
	)

	state := manager.Snapshot(context.Background())[0]
	if state.ServiceRunning || state.ConnectionState != "inactive" || state.StatusError != "" {
		t.Fatalf("tailscale state = %#v", state)
	}
}

func TestNetBirdRunningWithoutJSONSocketIsHonest(t *testing.T) {
	manager := newManager(
		fakeServices{states: map[string]serviceState{"netbird.service": {enabled: true, active: "active"}}},
		fakeProviderStatus{
			available: map[model.VPNProviderID]bool{model.VPNProviderNetBird: false},
			errs:      map[model.VPNProviderID]error{model.VPNProviderNetBird: errStatusUnavailable},
		},
		fakeInstalled{"netbird": true},
	)

	state := manager.Snapshot(context.Background())[1]
	if !state.Installed || !state.ServiceRunning || state.ConnectionState != "unavailable" {
		t.Fatalf("netbird state = %#v", state)
	}
	if state.Connected || len(state.Addresses) != 0 || state.StatusError != "" {
		t.Fatalf("optional NetBird status absence should not be an error: %#v", state)
	}
}

func TestProviderStatusFailureIsContained(t *testing.T) {
	manager := newManager(
		fakeServices{states: map[string]serviceState{"tailscaled.service": {enabled: true, active: "active"}}},
		fakeProviderStatus{
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
		fakeServices{errs: map[string]error{"tailscaled.service": errors.New("dbus unavailable")}},
		fakeProviderStatus{},
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

func TestNormalizeNetBirdStatus(t *testing.T) {
	status := netBirdGatewayStatus{Status: "Connected"}
	status.FullStatus = &struct {
		LocalPeerState *struct {
			IP      string `json:"IP"`
			IPLower string `json:"ip"`
			IPv6    string `json:"ipv6"`
		} `json:"localPeerState"`
	}{
		LocalPeerState: &struct {
			IP      string `json:"IP"`
			IPLower string `json:"ip"`
			IPv6    string `json:"ipv6"`
		}{IP: "100.119.62.6/16", IPv6: "fd00::6/64"},
	}

	state := normalizeNetBirdStatus(status)
	if !state.connected || state.state != "Connected" {
		t.Fatalf("netbird provider status = %#v", state)
	}
	want := []string{"100.119.62.6", "fd00::6"}
	if !reflect.DeepEqual(state.addresses, want) {
		t.Fatalf("addresses = %#v, want %#v", state.addresses, want)
	}
}
