# NetworkManager-HSP

NetworkManager-HSP (`nm-hsp`) is a first-party Home Server Project terminal application for friendly NetworkManager configuration on headless and appliance-style Linux systems.

The project is intended to provide a modern, keyboard-driven interface for ordinary Ethernet and Wi-Fi configuration while leaving the native NetworkManager `nmtui` available as the advanced fallback.

## Project status

Early development. Checkpoint 4 is implemented on the `testing` branch: `nm-hsp` now provides an interactive Ethernet settings form backed directly by NetworkManager D-Bus. Wi-Fi remains read-only until the next checkpoint.

## Goals

- Modern terminal UI suitable for users who are not Linux network administrators.
- Ethernet and Wi-Fi device discovery without requiring users to know interface names.
- DHCP and manual IPv4/IPv6 configuration.
- Gateway, DNS, autoconnect, MTU, and connection profile management.
- Wi-Fi scanning, signal/security display, hidden networks, saved profiles, and connection priority.
- Safe diagnostics and repair for common NetworkManager profile and autoconnect failures.
- NetworkManager D-Bus integration as the primary backend.
- Secret handling that avoids passwords in process arguments, logs, debug output, or shell history.
- Native `nmtui` remains available separately for uncommon or highly advanced NetworkManager configuration.

## Repository role

This repository owns the application source and releases.

A future RPM pipeline belongs in `home-server-project/home-server-packages`. Product repositories such as JustVoxel will consume the packaged application and may expose it through commands such as `mjust net`.

## Current backend contract

Checkpoint 2 uses NetworkManager's system D-Bus API directly. It currently reads:

- NetworkManager version, global state, connectivity, networking enabled, and Wi-Fi enabled state.
- Network devices, interface names, type, state, managed state, hardware address, and MTU.
- Ethernet carrier and speed reported by NetworkManager.
- Active Wi-Fi SSID, signal strength, and bitrate when available.
- Effective IPv4/IPv6 addresses and prefix lengths, gateway, and DNS servers.
- Saved connection profile ID, UUID, type, interface binding, autoconnect, and autoconnect priority.
- Active and available profile relationships for each device.

The backend reads NetworkManager directly over D-Bus and, for Ethernet only, can persist supported non-secret connection profile settings. It does not call `GetSecrets`.

For development and VM verification, `nm-hsp --snapshot` prints this normalized read-only state as JSON.

## Terminal dashboard

Running `nm-hsp` launches the terminal interface.

The dashboard currently provides:

- Overall NetworkManager connectivity and version.
- Networking and Wi-Fi enabled state.
- Friendly Ethernet and Wi-Fi device cards.
- Connection state, active profile, IPv4 address, Ethernet carrier/speed, and active Wi-Fi SSID/signal.
- Expanded read-only details for MAC address, MTU, IPv4/IPv6 addresses, gateway, DNS, and autoconnect state.
- Responsive compact rendering for smaller terminals.

Controls:

- Up/Down arrows or `j`/`k` — move between devices.
- Enter on Ethernet — open the interactive Ethernet settings form.
- Enter on Wi-Fi — show or hide read-only Wi-Fi details.
- `d` — show or hide device details.
- `r` — refresh NetworkManager state.
- `q`, Esc, or Ctrl-C — exit.

## Interactive Ethernet settings

The Checkpoint 4 Ethernet form supports:

- IPv4 Automatic / Manual / Disabled.
- IPv4 address/prefix, gateway, and DNS.
- IPv6 Automatic / Manual / Disabled.
- IPv6 address/prefix, gateway, and DNS.
- Autoconnect On / Off.
- MTU, with blank/zero treated as automatic.
- Explicit Save and Cancel actions.

The form uses standard single-line TUI input fields. Tab, Up/Down, and Enter move through fields; Left/Right change option fields; typing edits the selected value field.

For an existing Ethernet connection, nm-hsp re-reads the full non-secret NetworkManager profile immediately before saving and patches only the supported fields so unrelated profile settings are preserved. If the device has no saved Ethernet profile, nm-hsp creates a new persistent DHCP-style profile bound to that interface.

Checkpoint 4 saves profiles to disk only. It deliberately does not activate, deactivate, reconnect, or reapply the live network connection. Wi-Fi configuration and all secret handling remain future checkpoints.

## Source layout

- `cmd/nm-hsp` — application entry point
- `internal/networkmanager` — NetworkManager integration
- `internal/model` — application-facing network state
- `internal/ui` — terminal user interface
- `internal/security` — secret handling and safety helpers
- `internal/validation` — network input validation

## Development workflow

Development happens on `testing` in small reviewed checkpoints. `main` is reserved for reviewed/stable changes.

Current checkpoint sequence:

1. Repository foundation
2. NetworkManager backend and device discovery
3. Read-only modern TUI
4. Interactive Ethernet settings
5. Wi-Fi manager
6. Secret-handling audit
7. Diagnostics and repair
8. Release readiness and VM validation

## Build

Requires Go 1.25 or newer.

Run the standard Go validation locally with `go fmt`, `go vet`, `go test`, and `go build`.

## License

Apache License 2.0. See `LICENSE`.
