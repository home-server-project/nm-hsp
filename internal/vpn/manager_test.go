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
		t.Fatalf("disabled service should not be an error: %q", state.StatusError" —Ğ§Ğ ¦gVæ2FW7D–æ7F—fU6W'f–6TFöW4æ÷EVW'•&÷f–FW"‡B§FW7F–æråB’° —'VææW"£Òff¶U'VææW'° —F‡3¢Ö·7G&–æuÖ&ööÇ²'F–Ç66ÆR#¢G'VWÒÀ —&W7VÇG3¢Ö·7G&–æuÖf¶U&W7VÇG° ’'7—7FVÖ7FÂ—2ÖVæ&ÆVBF–Ç66ÆVBç6W'f–6R#¢¶÷WGWC¢&F—6&ÆVEÆâ"ÂW'#¢W'&÷'2äæWr‚&W†—B7FGW2"’—ÒÀ ’'7—7FVÖ7FÂ—2Ö7F—fRF–Ç66ÆVBç6W'f–6R#¢¶÷WGWC¢&–æ7F—fUÆâ"ÂW'#¢W'&÷'2äæWr‚&W†—B7FGW22"’—ÒÀ —ÒÀ —Ğ  —7FFR£ÒæWtÖævW"‡'VææW"’å6æ6†÷B†6öçFW‡Bä&6¶w&÷VæB‚’•³Ğ ––b7FFRå6W'f–6U'Vææ–ærÇÂ7FFRä6öææV7F–öå7FFRÒ&–æ7F—fR"° —BäfFÆb‚'F–Ç66ÆR7FFRÒR7b"Â7FFR —Ğ –f÷"òÂ6ÆÂ£Ò&ævR'VææW"æ6ÆÇ2° ––b6ÆÂÓÒ'F–Ç66ÆR7FGW2ÒÖ§6öâ"° —BäfFÂ‚&–æ7F—fR6W'f–6R6†÷VÆBæ÷BVW'’&÷f–FW"4Ä’" —Ğ —Ğ§Ğ ¦gVæ2FW7E&÷f–FW%7FGW4f–ÇW&T—46öçF–æVB‡B§FW7F–æråB’° —'VææW"£Òff¶U'VææW'° —F‡3¢Ö·7G&–æuÖ&ööÇ²'F–Ç66ÆR#¢G'VWÒÀ —&W7VÇG3¢Ö·7G&–æuÖf¶U&W7VÇG° ’'7—7FVÖ7FÂ—2ÖVæ&ÆVBF–Ç66ÆVBç6W'f–6R#¢¶÷WGWC¢&Væ&ÆVEÆâ'ÒÀ ’'7—7FVÖ7FÂ—2Ö7F—fRF–Ç66ÆVBç6W'f–6R#¢¶÷WGWC¢&7F—fUÆâ'ÒÀ ’'F–Ç66ÆR7FGW2ÒÖ§6öâ#¢¶÷WGWC¢&æ÷BÖ§6öâ"ÂW'#¢W'&÷'2äæWr‚&W†—B7FGW2"—ÒÀ —ÒÀ —Ğ  —7FFW2£ÒæWtÖævW"‡'VææW"’å6æ6†÷B†6öçFW‡Bä&6¶w&÷VæB‚’ ––b7FFW5³Òä6öææV7F–öå7FFRÒ'Væ¶æ÷vâ"ÇÂ7FFW5³Òå7FGW4W'&÷"ÓÒ""° —BäfFÆb‚'F–Ç66ÆRf–ÇW&R7FFRÒR7b"Â7FFW5³Ò —Ğ ––b7FFW5³Òä”BÒÖöFVÂåeå&÷f–FW$æWD&—&B° —BäfFÆb‚'&VÖ–æ–ær&÷f–FW"v2æ÷B6öÆÆV7FVC¢R7b"Â7FFW2 —Ğ§Ğ ¦gVæ2FW7E'6TæWD&—&E7FGW4fÆÇ4&6µFôÖævVÖVçE7FFR‡B§FW7F–æråB’° —7FFRÂ6öææV7FVBÂFG&W76W2ÂW'"£Ò'6TæWD&—&E7FGW2…µÖ'—FR†²&ÖævVÖVçB#§²&6öææV7FVB#§G'VWÒÂ&æWF&—&D—#¢#ãcBãã#ób'Ö’ ––bW'"Òæ–Â° —BäfFÂ†W'" —Ğ ––b7FFRÒ$6öææV7FVB"ÇÂ6öææV7FVB° —BäfFÆb‚'7FFRÒW6öææV7FVBÒWb"Â7FFRÂ6öææV7FVB —Ğ ––b&VfÆV7BäFVWWVÂ†FG&W76W2Âµ×7G&–æw²#ãcBãã#'Ò’° —BäfFÆb‚&FG&W76W2ÒR7b"ÂFG&W76W2 —Ğ§Ğ 