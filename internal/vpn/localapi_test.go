// SPDX-License-Identifier: Apache-2.0

package vpn

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTailscaleLocalAPIRequestUsesRequiredHost(t *testing.T) {
	req, err := newUnixJSONRequest(
		context.Background(),
		http.MethodGet,
		"/localapi/v0/status",
		io.NopCloser(strings.NewReader("")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if req.Host != tailscaleLocalAPIHost {
		t.Fatalf("Host = %q, want %q", req.Host, tailscaleLocalAPIHost)
	}
	if req.URL.Host != tailscaleLocalAPIHost {
		t.Fatalf("URL host = %q, want %q", req.URL.Host, tailscaleLocalAPIHost)
	}
}
