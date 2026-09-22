// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/home-server-project/nm-hsp/internal/model"
)

const (
	tailscaleSocket   = "/var/run/tailscale/tailscaled.sock"
	maxStatusResponse = 2 << 20
)

var errStatusUnavailable = errors.New("provider status interface unavailable")

type localProviderBackend struct {
	netbird netBirdRPC
}

func newLocalProviderBackend() *localProviderBackend {
	return &localProviderBackend{netbird: netBirdRPC{}}
}

func (b *localProviderBackend) Status(ctx context.Context, provider model.VPNProviderID) (providerStatus, bool, error) {
	switch provider {
	case model.VPNProviderTailscale:
		return readTailscaleStatus(ctx)
	case model.VPNProviderNetBird:
		return b.netbird.Status(ctx)
	default:
		return providerStatus{}, false, errStatusUnavailable
	}
}

func (b *localProviderBackend) Connect(ctx context.Context, provider model.VPNProviderID) (providerActionResult, error) {
	switch provider {
	case model.VPNProviderTailscale:
		return connectTailscale(ctx)
	case model.VPNProviderNetBird:
		return b.netbird.Connect(ctx)
	default:
		return providerActionResult{}, errStatusUnavailable
	}
}

func (b *localProviderBackend) Disconnect(ctx context.Context, provider model.VPNProviderID) error {
	switch provider {
	case model.VPNProviderTailscale:
		return setTailscaleWantRunning(ctx, false)
	case model.VPNProviderNetBird:
		return b.netbird.Disconnect(ctx)
	default:
		return errStatusUnavailable
	}
}

func (b *localProviderBackend) WaitAuthentication(ctx context.Context, provider model.VPNProviderID, userCode string) (providerActionResult, error) {
	switch provider {
	case model.VPNProviderTailscale:
		return waitForTailscaleAuthentication(ctx)
	case model.VPNProviderNetBird:
		return b.netbird.WaitAuthentication(ctx, userCode)
	default:
		return providerActionResult{}, errStatusUnavailable
	}
}

type tailscaleStatus struct {
	BackendState string   `json:"BackendState"`
	AuthURL      string   `json:"AuthURL"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

func readTailscaleStatus(ctx context.Context) (providerStatus, bool, error) {
	var status tailscaleStatus
	if err := queryUnixJSON(ctx, tailscaleSocket, http.MethodGet, "/localapi/v0/status", nil, &status); err != nil {
		if errors.Is(err, errStatusUnavailable) {
			return providerStatus{}, false, err
		}
		return providerStatus{}, true, err
	}
	return normalizeTailscaleStatus(status), true, nil
}

func normalizeTailscaleStatus(status tailscaleStatus) providerStatus {
	state := strings.TrimSpace(status.BackendState)
	if state == "" {
		state = "unknown"
	}
	return providerStatus{
		state:     state,
		connected: state == "Running",
		addresses: normalizeAddresses(status.TailscaleIPs),
		authURL:   strings.TrimSpace(status.AuthURL),
	}
}

func connectTailscale(ctx context.Context) (providerActionResult, error) {
	if err := setTailscaleWantRunning(ctx, true); err != nil {
		return providerActionResult{}, err
	}

	state, _, err := readTailscaleStatus(ctx)
	if err != nil {
		return providerActionResult{}, err
	}
	if state.connected {
		return providerActionResult{message: "Tailscale connected."}, nil
	}

	if state.authURL == "" && strings.EqualFold(state.state, "NeedsLogin") {
		if err := postTailscaleLoginInteractive(ctx); err != nil {
			return providerActionResult{}, err
		}
		state, err = waitForTailscaleAuthURL(ctx)
		if err != nil {
			return providerActionResult{}, err
		}
	}

	if state.connected {
		return providerActionResult{message: "Tailscale connected."}, nil
	}
	if state.authURL != "" {
		return providerActionResult{
			message:      "Open the Tailscale login URL to finish authentication.",
			authURL:      state.authURL,
			awaitingAuth: true,
		}, nil
	}
	return providerActionResult{message: "Tailscale connection requested."}, nil
}

func setTailscaleWantRunning(ctx context.Context, running bool) error {
	body, err := json.Marshal(map[string]any{
		"WantRunning":    running,
		"WantRunningSet": true,
	})
	if err != nil {
		return err
	}
	var response map[string]any
	return queryUnixJSON(ctx, tailscaleSocket, http.MethodPatch, "/localapi/v0/prefs", body, &response)
}

func postTailscaleLoginInteractive(ctx context.Context) error {
	return queryUnixJSON(ctx, tailscaleSocket, http.MethodPost, "/localapi/v0/login-interactive", nil, nil)
}

func waitForTailscaleAuthURL(ctx context.Context) (providerStatus, error) {
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		state, _, err := readTailscaleStatus(ctx)
		if err == nil && (state.connected || state.authURL != "") {
			return state, nil
		}

		select {
		case <-ctx.Done():
			return providerStatus{}, ctx.Err()
		case <-timeout.C:
			return providerStatus{}, errors.New("Tailscale did not provide a login URL")
		case <-ticker.C:
		}
	}
}

func waitForTailscaleAuthentication(ctx context.Context) (providerActionResult, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		state, _, err := readTailscaleStatus(ctx)
		if err == nil && state.connected {
			return providerActionResult{message: "Tailscale authentication completed and the connection is active."}, nil
		}

		select {
		case <-ctx.Done():
			return providerActionResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func queryUnixJSON(ctx context.Context, socketPath, method, requestPath string, body []byte, out any) error {
	info, err := os.Stat(socketPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errStatusUnavailable
		}
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("local API path is not a socket")
	}

	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	defer transport.CloseIdleConnections()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+requestPath, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("local API returned HTTP %d", resp.StatusCode)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil
	}

	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxStatusResponse))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode local API response: %w", err)
	}
	return nil
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
