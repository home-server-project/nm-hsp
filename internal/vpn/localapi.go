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
	netBirdJSONSocket = "/var/run/netbird-http.sock"
	maxStatusResponse = 2 << 20
)

var errStatusUnavailable = errors.New("provider status interface unavailable")

type localAPIReader struct{}

func (localAPIReader) Status(ctx context.Context, provider model.VPNProviderID) (providerStatus, bool, error) {
	switch provider {
	case model.VPNProviderTailscale:
		return readTailscaleStatus(ctx)
	case model.VPNProviderNetBird:
		return readNetBirdStatus(ctx)
	default:
		return providerStatus{}, false, errStatusUnavailable
	}
}

type tailscaleStatus struct {
	BackendState string   `json:"BackendState"`
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
	}
}

type netBirdGatewayStatus struct {
	Status     string `json:"status"`
	FullStatus *struct {
		LocalPeerState *struct {
			IP      string `json:"IP"`
			IPLower string `json:"ip"`
			IPv6    string `json:"ipv6"`
		} `json:"localPeerState"`
	} `json:"fullStatus"`
}

func readNetBirdStatus(ctx context.Context) (providerStatus, bool, error) {
	body := []byte(`{"getFullPeerStatus":true,"shouldRunProbes":false}`)
	var status netBirdGatewayStatus
	if err := queryUnixJSON(ctx, netBirdJSONSocket, http.MethodPost, "/daemon.DaemonService/Status", body, &status); err != nil {
		if errors.Is(err, errStatusUnavailable) {
			return providerStatus{}, false, err
		}
		return providerStatus{}, true, err
	}
	return normalizeNetBirdStatus(status), true, nil
}

func normalizeNetBirdStatus(status netBirdGatewayStatus) providerStatus {
	state := strings.TrimSpace(status.Status)
	if state == "" {
		state = "unknown"
	}

	var addresses []string
	if status.FullStatus != nil && status.FullStatus.LocalPeerState != nil {
		ipv4 := status.FullStatus.LocalPeerState.IP
		if ipv4 == "" {
			ipv4 = status.FullStatus.LocalPeerState.IPLower
		}
		addresses = normalizeAddresses([]string{ipv4, status.FullStatus.LocalPeerState.IPv6})
	}

	return providerStatus{
		state:     state,
		connected: strings.EqualFold(state, "Connected"),
		addresses: addresses,
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

	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxStatusResponse))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode local API status: %w", err)
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
