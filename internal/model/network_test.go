// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

func TestDeviceKindFromNM(t *testing.T) {
	tests := []struct {
		name string
		in   uint32
		want DeviceKind
	}{
		{name: "ethernet", in: 1, want: DeviceKindEthernet},
		{name: "wifi", in: 2, want: DeviceKindWiFi},
		{name: "unknown", in: 999, want: DeviceKindOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeviceKindFromNM(tt.in); got != tt.want {
				t.Fatalf("DeviceKindFromNM(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDeviceStateName(t *testing.T) {
	if got := DeviceStateName(100); got != "activated" {
		t.Fatalf("DeviceStateName(100) = %q, want activated", got)
	}
	if got := DeviceStateName(120); got != "failed" {
		t.Fatalf("DeviceStateName(120) = %q, want failed", got)
	}
	if got := DeviceStateName(999); got != "unknown" {
		t.Fatalf("DeviceStateName(999) = %q, want unknown", got)
	}
}

func TestConnectivityName(t *testing.T) {
	if got := ConnectivityName(4); got != "full" {
		t.Fatalf("ConnectivityName(4) = %q, want full", got)
	}
	if got := ConnectivityName(3); got != "limited" {
		t.Fatalf("ConnectivityName(3) = %q, want limited", got)
	}
	if got := ConnectivityName(0); got != "unknown" {
		t.Fatalf("ConnectivityName(0) = %q, want unknown", got)
	}
}
