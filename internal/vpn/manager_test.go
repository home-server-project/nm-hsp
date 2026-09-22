// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/home-server-project/nm-hsp/internal/model"
)

type fakeResult struct {
	output string
	err    error
}

type fakeRunner struct {
	paths   map[string]bool
	results map[string]fakeResult
	calls   []string
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if f.paths[name] {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	result, ok := f.results[key]
	if !ok {
		return nil, errors.New("unexpected command: " + key)
	}
	return []byte(result.output), result.err
}

func TestSnapshotAlwaysListsSupportedProviders(t *testing.T) {
	runner := &fakeRunner{paths: map[string]bool{}, results: map[string]fakeResult{}}
	states := newManager(runner).Snapshot(context.Background())

	if len(states) != 2 {
		t.Fatalf("provider count = %d, want 2", len(states))
	}
	if states[0].ID != model.VPNProviderTailscale || states[0].Installed {
		t.Fatalf("tailscale state = %#v", states[0])
	}
	if states[1].ID != model.VPNProviderNetBird || states[1].Installed {
		t.Fatalf("netbird state = %#v", states[1])
	}
	if len(runner.calls) != 0 {
		t.Fatalf("uninstalled providers executed commands: %#v", runner.calls)
	}
}

func TestTailscaleConnectedState(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]bool{"tailscale": true},
		results: map[string]fakeResult{
			"systemctl is-enabled tailscaled.service": {output: "enabled\n"},
			"systemctl is-active tailscaled.service":  {output: "active\n"},
			"tailscale status --json":                 {output: `{"BackendState":"Running","TailscaleIPs":["100.64.0.10","fd7a:115c:a1e0::10"]}`},
		},
	}

	state := newManager(runner).Snapshot(context.Background())[0]
	if !state.Installed || !state.ServiceEnabled || !state.ServiceRunning || !state.Connected {
		t.Fatalf("tailscale state = %#v", state)
	}
	if state.ConnectionState != "Running" {
		t.Fatalf("connection state = %q, want Running", state.ConnectionState)
	}
	wantAddresses := []string{"100.64.0.10", "fd7a:115c:a1e0::10"}
	if !reflect.DeepEqual(state.Addresses, wantAddresses) {
		t.Fatalf("addresses = %#v, want %#v", state.Addresses, wantAddresses)
	}
}

func TestNetBirdConnectedStateNormalizesCIDRs(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]bool{"netbird": true},
		results: map[string]fakeResult{
			"systemctl is-enabled netbird.service": {output: "disabled\n", err: errors.New("exit status 1")},
			"systemctl is-active netbird.service":  {output: "active\n"},
			"netbird status --json":                {output: `{"daemonStatus":"Connected","management":{"connected":true},"netbirdIp":"100.119.62.6/16","netbirdIpv6":"fd00::6/64"}`},
		},
	}

	state := newManager(runner).Snapshot(context.Background())[1]
	if !state.Installed || state.ServiceEnabled || !state.ServiceRunning || !state.Connected {
		t.Fatalf("netbird state = %#v", state)
	}
	wantAddresses := []string{"100.119.62.6", "fd00::6"}
	if !reflect.DeepEqual(state.Addresses, wantAddresses) {
		t.Fatalf("addresses = %#v, want %#v", state.Addresses, wantAddresses)
	}
	if state.StatusError != "" {
		t.Fatalf("disabled service should not be an error: %q", state.StatusError)
	}
}

func TestInactiveServiceDoesNotQueryProvider(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]bool{"tailscale": true},
		results: map[string]fakeResult{
			"systemctl is-enabled tailscaled.service": {output: "disabled\n", err: errors.New("exit status 1")},
			"systemctl is-active tailscaled.service":  {output: "inactive\n", err: errors.New("exit status 3")},
		},
	}

	state := newManager(runner).Snapshot(context.Background())[0]
	if state.ServiceRunning || state.ConnectionState != "inactive" {
		t.Fatalf("tailscale state = %#v", state)
	}
	for _, call := range runner.calls {
		if call == "tailscale status --json" {
			t.Fatal("inactive service should not query provider CLI")
		}
	}
}

func TestProviderStatusFailureIsContained(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]bool{"tailscale": true},
		results: map[string]fakeResult{
			"systemctl is-enabled tailscaled.service": {output: "enabled\n"},
			"systemctl is-active tailscaled.service":  {output: "active\n"},
			"tailscale status --json":                 {output: "not-json", err: errors.New("exit status 1")},
		},
	}

	states := newManager(runner).Snapshot(context.Background())
	if states[0].ConnectionState != "unknown" || states[0].StatusError == "" {
		t.Fatalf("tailscale failure state = %#v", states[0])
	}
	if states[1].ID != model.VPNProviderNetBird {
		t.Fatalf("remaining provider was not collected: %#v", states)
	}
}

func TestParseNetBirdStatusFallsBackToManagementState(t *testing.T) {
	state, connected, addresses, err := parseNetBirdStatus([]byte(`{"management":{"connected":true},"netbirdIp":"100.64.10.20/16"}`))
	if err != nil {
		t.Fatal(err)
	}
	if state != "Connected" || !connected {
		t.Fatalf("state = %q connected = %v", state, connected)
	}
	if !reflect.DeepEqual(addresses, []string{"100.64.10.20"}) {
		t.Fatalf("addresses = %#v", addresses)
	}
}
