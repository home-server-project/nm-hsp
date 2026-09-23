// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWaitForServiceStateReturnsOnRequestedState(t *testing.T) {
	err := waitForServiceState(
		context.Background(),
		"tailscaled.service",
		"active",
		func(context.Context, string) (serviceState, error) {
			return serviceState{active: "active"}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWaitForServiceStateReturnsReadErrorImmediately(t *testing.T) {
	want := errors.New("dbus read failed")
	start := time.Now()

	err := waitForServiceState(
		context.Background(),
		"tailscaled.service",
		"active",
		func(context.Context, string) (serviceState, error) {
			return serviceState{}, want
		},
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("state read error took too long: %v", elapsed)
	}
}

func TestWaitForServiceStateReturnsFailedStateImmediately(t *testing.T) {
	start := time.Now()

	err := waitForServiceState(
		context.Background(),
		"tailscaled.service",
		"active",
		func(context.Context, string) (serviceState, error) {
			return serviceState{active: "failed"}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "failed state") {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("failed state took too long: %v", elapsed)
	}
}

func TestWaitForServiceStateHonorsContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := waitForServiceState(
		ctx,
		"netbird.service",
		"active",
		func(context.Context, string) (serviceState, error) {
			return serviceState{active: "activating"}, nil
		},
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("deadline handling took too long: %v", elapsed)
	}
}
