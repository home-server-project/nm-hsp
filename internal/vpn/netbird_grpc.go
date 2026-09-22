// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const netBirdSocket = "/var/run/netbird.sock"

type netBirdRPC struct{}

func (netBirdRPC) Status(ctx context.Context) (providerStatus, bool, error) {
	conn, err := dialNetBird()
	if err != nil {
		if errors.Is(err, errStatusUnavailable) {
			return providerStatus{}, false, err
		}
		return providerStatus{}, true, err
	}
	defer conn.Close()

	var response nbStatusResponse
	if err := conn.Invoke(ctx, "/daemon.DaemonService/Status", &nbStatusRequest{GetFullPeerStatus: true}, &response); err != nil {
		return providerStatus{}, true, err
	}
	return normalizeNetBirdRPCStatus(response), true, nil
}

func (n netBirdRPC) Connect(ctx context.Context) (providerActionResult, error) {
	conn, err := dialNetBird()
	if err != nil {
		return providerActionResult{}, err
	}
	defer conn.Close()

	var login nbLoginResponse
	if err := conn.Invoke(ctx, "/daemon.DaemonService/Login", &nbLoginRequest{}, &login); err != nil {
		return providerActionResult{}, err
	}

	if login.NeedsSSOLogin {
		url := strings.TrimSpace(login.VerificationURIComplete)
		if url == "" {
			url = strings.TrimSpace(login.VerificationURI)
		}
		if url == "" {
			return providerActionResult{}, errors.New("NetBird requested SSO but did not provide a verification URL")
		}
		return providerActionResult{
			message:      "Open the NetBird verification URL to finish authentication.",
			authURL:      url,
			userCode:     strings.TrimSpace(login.UserCode),
			awaitingAuth: true,
		}, nil
	}

	if err := invokeNetBirdUp(ctx, conn); err != nil {
		return providerActionResult{}, err
	}
	return providerActionResult{message: "NetBird connection requested."}, nil
}

func (netBirdRPC) Disconnect(ctx context.Context) error {
	conn, err := dialNetBird()
	if err != nil {
		return err
	}
	defer conn.Close()

	return conn.Invoke(ctx, "/daemon.DaemonService/Down", &nbDownRequest{}, &nbDownResponse{})
}

func (n netBirdRPC) WaitAuthentication(ctx context.Context, userCode string) (providerActionResult, error) {
	if strings.TrimSpace(userCode) == "" {
		return providerActionResult{}, errors.New("NetBird authentication code is missing")
	}

	conn, err := dialNetBird()
	if err != nil {
		return providerActionResult{}, err
	}
	defer conn.Close()

	var waitResponse nbWaitSSOLoginResponse
	if err := conn.Invoke(
		ctx,
		"/daemon.DaemonService/WaitSSOLogin",
		&nbWaitSSOLoginRequest{UserCode: userCode},
		&waitResponse,
	); err != nil {
		return providerActionResult{}, err
	}
	if err := invokeNetBirdUp(ctx, conn); err != nil {
		return providerActionResult{}, err
	}

	if err := waitForNetBirdConnected(ctx, n); err != nil {
		return providerActionResult{}, err
	}
	return providerActionResult{message: "NetBird authentication completed and the connection is active."}, nil
}

func invokeNetBirdUp(ctx context.Context, conn *grpc.ClientConn) error {
	return conn.Invoke(ctx, "/daemon.DaemonService/Up", &nbUpRequest{Async: true}, &nbUpResponse{})
}

func waitForNetBirdConnected(ctx context.Context, n netBirdRPC) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		state, _, err := n.Status(ctx)
		if err == nil && state.connected {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func normalizeNetBirdRPCStatus(status nbStatusResponse) providerStatus {
	state := strings.TrimSpace(status.Status)
	if state == "" {
		state = "unknown"
	}

	var addresses []string
	if status.FullStatus != nil && status.FullStatus.LocalPeerState != nil {
		addresses = normalizeAddresses([]string{
			status.FullStatus.LocalPeerState.IP,
			status.FullStatus.LocalPeerState.IPv6,
		})
	}

	return providerStatus{
		state:     state,
		connected: strings.EqualFold(state, "Connected"),
		addresses: addresses,
	}
}

func dialNetBird() (*grpc.ClientConn, error) {
	socket, err := resolveNetBirdSocket()
	if err != nil {
		return nil, err
	}
	return grpc.NewClient(
		"unix://"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

func resolveNetBirdSocket() (string, error) {
	if info, err := os.Stat(netBirdSocket); err == nil && info.Mode()&os.ModeSocket != 0 {
		return netBirdSocket, nil
	}

	entries, err := os.ReadDir("/var/run/netbird")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errStatusUnavailable
		}
		return "", err
	}

	var sockets []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sock") {
			continue
		}
		path := filepath.Join("/var/run/netbird", entry.Name())
		info, err := os.Stat(path)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			sockets = append(sockets, path)
		}
	}

	if len(sockets) == 1 {
		return sockets[0], nil
	}
	if len(sockets) == 0 {
		return "", errStatusUnavailable
	}
	return "", fmt.Errorf("multiple NetBird daemon sockets found")
}

// The minimal message types below mirror only the stable fields nm-hsp needs
// from NetBird's daemon.proto. gRPC's protobuf codec accepts MessageV1 values,
// so nm-hsp does not need to vendor NetBird's full client module.

type nbLoginRequest struct{}

func (*nbLoginRequest) Reset()         {}

func (*nbLoginRequest) String() string { return "LoginRequest{}" }

func (*nbLoginRequest) ProtoMessage()  {}

type nbLoginResponse struct {
	NeedsSSOLogin           bool   `protobuf:"varint,1,opt,name=needsSSOLogin,proto3" json:"needsSSOLogin,omitempty"`
	UserCode                string `protobuf:"bytes,2,opt,name=userCode,proto3" json:"userCode,omitempty"`
	VerificationURI         string `protobuf:"bytes,3,opt,name=verificationURI,proto3" json:"verificationURI,omitempty"`
	VerificationURIComplete string `protobuf:"bytes,4,opt,name=verificationURIComplete,proto3" json:"verificationURIComplete,omitempty"`
}

func (*nbLoginResponse) Reset()         {}

func (*nbLoginResponse) String() string { return "LoginResponse{}" }

func (*nbLoginResponse) ProtoMessage()  {}

type nbWaitSSOLoginRequest struct {
	UserCode string `protobuf:"bytes,1,opt,name=userCode,proto3" json:"userCode,omitempty"`
}

func (*nbWaitSSOLoginRequest) Reset()         {}

func (*nbWaitSSOLoginRequest) String() string { return "WaitSSOLoginRequest{}" }

func (*nbWaitSSOLoginRequest) ProtoMessage()  {}

type nbWaitSSOLoginResponse struct {
	Email string `protobuf:"bytes,1,opt,name=email,proto3" json:"email,omitempty"`
}

func (*nbWaitSSOLoginResponse) Reset()         {}

func (*nbWaitSSOLoginResponse) String() string { return "WaitSSOLoginResponse{}" }

func (*nbWaitSSOLoginResponse) ProtoMessage()  {}

type nbUpRequest struct {
	Async bool `protobuf:"varint,4,opt,name=async,proto3" json:"async,omitempty"`
}

func (*nbUpRequest) Reset()         {}

func (*nbUpRequest) String() string { return "UpRequest{}" }

func (*nbUpRequest) ProtoMessage()  {}

type nbUpResponse struct{}

func (*nbUpResponse) Reset()         {}

func (*nbUpResponse) String() string { return "UpResponse{}" }

func (*nbUpResponse) ProtoMessage()  {}

type nbDownRequest struct{}

func (*nbDownRequest) Reset()         {}

func (*nbDownRequest) String() string { return "DownRequest{}" }

func (*nbDownRequest) ProtoMessage()  {}

type nbDownResponse struct{}

func (*nbDownResponse) Reset()         {}

func (*nbDownResponse) String() string { return "DownResponse{}" }

func (*nbDownResponse) ProtoMessage()  {}

type nbStatusRequest struct {
	GetFullPeerStatus bool `protobuf:"varint,1,opt,name=getFullPeerStatus,proto3" json:"getFullPeerStatus,omitempty"`
}

func (*nbStatusRequest) Reset()         {}

func (*nbStatusRequest) String() string { return "StatusRequest{}" }

func (*nbStatusRequest) ProtoMessage()  {}

type nbStatusResponse struct {
	Status     string        `protobuf:"bytes,1,opt,name=status,proto3" json:"status,omitempty"`
	FullStatus *nbFullStatus `protobuf:"bytes,2,opt,name=fullStatus,proto3" json:"fullStatus,omitempty"`
}

func (*nbStatusResponse) Reset()         {}

func (*nbStatusResponse) String() string { return "StatusResponse{}" }

func (*nbStatusResponse) ProtoMessage()  {}

type nbFullStatus struct {
	LocalPeerState *nbLocalPeerState `protobuf:"bytes,3,opt,name=localPeerState,proto3" json:"localPeerState,omitempty"`
}

func (*nbFullStatus) Reset()         {}

func (*nbFullStatus) String() string { return "FullStatus{}" }

func (*nbFullStatus) ProtoMessage()  {}

type nbLocalPeerState struct {
	IP   string `protobuf:"bytes,1,opt,name=IP,proto3" json:"IP,omitempty"`
	IPv6 string `protobuf:"bytes,8,opt,name=ipv6,proto3" json:"ipv6,omitempty"`
}

func (*nbLocalPeerState) Reset()         {}

func (*nbLocalPeerState) String() string { return "LocalPeerState{}" }

func (*nbLocalPeerState) ProtoMessage()  {}
