// SPDX-License-Identifier: Apache-2.0

package diagnostics

import (
	"context"
	"net"

	"github.com/home-server-project/nm-hsp/internal/model"
)

const dnsProbeName = "example.com"

// ProbeDNS performs one ordinary resolver lookup. It does not use raw sockets,
// shell commands, or a hard-coded public DNS server.
func ProbeDNS(ctx context.Context) model.DiagnosticProbe {
	result := model.DiagnosticProbe{DNSAttempted: true}
	_, err := net.DefaultResolver.LookupHost(ctx, dnsProbeName)
	if err != nil {
		result.DNSError = err.Error()
		return result
	}
	result.DNSWorking = true
	return result
}
