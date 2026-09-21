# NetworkManager-HSP

NetworkManager-HSP (`nm-hsp`) is a first-party Home Server Project terminal application for friendly NetworkManager configuration on headless and appliance-style Linux systems.

The project is intended to provide a modern, keyboard-driven interface for ordinary Ethernet and Wi-Fi configuration while leaving the native NetworkManager `nmtui` available as the advanced fallback.

## Project status

Early development. Checkpoint 2 is implemented on the `testing` branch: a read-only NetworkManager D-Bus backend discovers devices, runtime IP configuration, connectivity state, and non-secret connection profile metadata. The terminal UI and all network-changing operations remain future checkpoints.

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

The backend does not call NetworkManager mutation methods and does not call `GetSecrets`.

For development and VM verification, `nm-hsp --snapshot` prints this normalized read-only state as JSON.

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
4. Ethernet editor
5. Wi-Fi manager
6. Secret-handling audit
7. Diagnostics and repair
8. Release readiness and VM validation

## Build

Requires Go 1.25 or newer.

Run the standard Go validation locally with `go fmt`, `go vet`, `go test`, and `go build`.

## License

Apache License 2.0. See `LICENSE`.
