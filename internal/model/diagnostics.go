// SPDX-License-Identifier: Apache-2.0

package model

// DiagnosticStatus is the severity/state of a diagnostic check.
type DiagnosticStatus string

const (
	DiagnosticOK      DiagnosticStatus = "ok"
	DiagnosticInfo    DiagnosticStatus = "info"
	DiagnosticWarning DiagnosticStatus = "warning"
	DiagnosticProblem DiagnosticStatus = "problem"
)

// RepairKind identifies a narrowly scoped repair that nm-hsp knows how to
// perform without guessing at user intent.
type RepairKind string

const (
	RepairCreateDHCPProfile   RepairKind = "create-dhcp-profile"
	RepairEnableAutoconnect   RepairKind = "enable-autoconnect"
	RepairActivateProfile     RepairKind = "activate-profile"
	RepairFixInterfaceBinding RepairKind = "fix-interface-binding"
	RepairEnableWiFi          RepairKind = "enable-wifi"
)

// RepairAction describes an explicit before/after change offered to the user.
type RepairAction struct {
	Kind          RepairKind
	Label         string
	Description   string
	DevicePath    string
	InterfaceName string
	ProfilePath   string
	ProfileID     string
	Before        string
	After         string
}

// DiagnosticCheck is one user-facing result in the network health chain.
type DiagnosticCheck struct {
	ID         string
	Interface  string
	DevicePath string
	Status     DiagnosticStatus
	Title      string
	Detail     string
	Repair     *RepairAction
}

// DiagnosticProbe contains optional active checks that are not available from
// NetworkManager's state alone.
type DiagnosticProbe struct {
	DNSAttempted bool
	DNSWorking   bool
	DNSError     string
}

// DiagnosticReport is the complete diagnostics result.
type DiagnosticReport struct {
	Checks []DiagnosticCheck
}
