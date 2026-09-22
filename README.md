# NetworkManager-HSP

NetworkManager-HSP (`nm-hsp`) is a first-party Home Server Project terminal application for friendly NetworkManager configuration on headless and appliance-style Linux systems.

The project is intended to provide a modern-friendly, keyboard-driven interface for ordinary Ethernet and Wi-Fi configuration.

## Project status

Early development. `nm-hsp` includes interactive Ethernet and Wi-Fi management, hardened secret handling, and a friendly network diagnostics and guarded repair workflow.

## Goals

- Modern terminal UI suitable for users who are not Linux network administrators.
- Ethernet and Wi-Fi device discovery without requiring users to know interface names.
- DHCP and manual IPv4/IPv6 configuration.
- Gateway, DNS, autoconnect, MTU, and connection profile management.
- Wi-Fi scanning, signal/security display, hidden networks, saved profiles, and connection priority.
- Safe diagnostics and repair for common NetworkManager profile and autoconnect failures.
- NetworkManager D-Bus integration as the primary backend.
- Secret handling that avoids passwords in process arguments, logs, debug output, or shell history.

## Repository role

This repository owns the application source and releases.

A future RPM pipeline belongs in `home-server-project/home-server-packages`. Product repositories such as [JustVoxel](https://github.com/home-server-project/justvoxel) will consume the packaged application.

## Current backend contract

nm-hsp uses NetworkManager's system D-Bus API directly. It currently reads:

- NetworkManager version, global state, connectivity, networking enabled, and Wi-Fi enabled state.
- Network devices, interface names, type, state, managed state, hardware address, and MTU.
- Ethernet carrier and speed reported by NetworkManager.
- Active Wi-Fi SSID, signal strength, and bitrate when available.
- Effective IPv4/IPv6 addresses and prefix lengths, gateway, and DNS servers.
- Saved connection profile ID, UUID, type, interface binding, autoconnect, and autoconnect priority.
- Active and available profile relationships for each device.

The backend reads and updates NetworkManager directly over D-Bus. It can persist supported Ethernet settings, scan Wi-Fi networks, toggle the Wi-Fi radio, activate/deactivate saved Wi-Fi profiles, create supported Wi-Fi profiles, update autoconnect metadata, and delete saved Wi-Fi profiles. It does not call `GetSecrets`.

For development and verification, `nm-hsp --snapshot` prints this normalized read-only state as JSON.

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
- Enter on Wi-Fi — open the interactive Wi-Fi manager.
- `t` — open Network health & repair.
- `d` — show or hide device details.
- `r` — refresh NetworkManager state.
- `q`, Esc, or Ctrl-C — exit.

## Interactive Ethernet settings

Ethernet management supports saved profiles, DHCP and manual IPv4/IPv6 configuration, DNS, gateway, autoconnect, MTU, connection activation, disconnection, and profile creation and editing.

The Ethernet settings form supports:

- IPv4 Automatic / Manual / Disabled.
- IPv4 address/prefix, gateway, and DNS.
- IPv6 Automatic / Manual / Disabled.
- IPv6 address/prefix, gateway, and DNS.
- Autoconnect On / Off.
- MTU, with blank/zero treated as automatic.
- Explicit Save and Cancel actions.

The form uses standard single-line TUI input fields. Tab, Up/Down, and Enter move through fields; Left/Right change option fields; typing edits the selected value field.

For an existing Ethernet connection, nm-hsp re-reads the full non-secret NetworkManager profile immediately before saving and patches only the supported fields so unrelated profile settings are preserved. If the device has no saved Ethernet profile, nm-hsp creates a new persistent DHCP-style profile bound to that interface.


## Interactive Wi-Fi manager

The Wi-Fi manager supports:

- Wi-Fi radio On / Off.
- Rescan of visible networks.
- SSID, signal, security type, band, known/saved state, and active state.
- Activation of existing saved Wi-Fi profiles.
- Disconnect of the active Wi-Fi connection.
- Forget/delete of saved Wi-Fi profiles.
- Hidden-network setup.
- Autoconnect On / Off and autoconnect priority for saved profiles.
- New Open, Enhanced Open (OWE), WPA/WPA2 Personal, and WPA3 Personal connections.
- Masked password input with an explicit Show Password toggle for new Personal connections.
- Saved profiles display `Password: unchanged`; nm-hsp does not fetch the existing saved password into the form.

New Enterprise and legacy WEP credential setup are intentionally not implemented yet. Existing saved Enterprise profiles can still be activated because NetworkManager already owns their credentials.

Wi-Fi connection creation and activation use NetworkManager D-Bus directly. Passwords are not passed through shell commands or process arguments, saved passwords are not fetched with `GetSecrets`, and new-password values are redacted from formatting and error paths. The visible form is cleared immediately when Connect is submitted or cancelled; copied `Secret` values share one clear-state; and the temporary D-Bus settings map drops its `psk` entry immediately after the call. See `docs/security.md` for the exact guarantees and limitations.


## Network health & repair

Press `t` from the dashboard to open the diagnostics screen.

The health view follows the network chain in normal-user language instead of requiring users to interpret NetworkManager internals. Depending on available hardware and state, it checks:

- Ethernet or Wi-Fi adapter presence.
- Whether NetworkManager manages the adapter.
- Ethernet carrier/cable state and reported link speed.
- Persistent Ethernet profile presence.
- Active versus inactive saved profiles.
- Autoconnect state.
- Saved interface-name binding.
- Saved Ethernet MAC binding versus the current adapter.
- Automatic IPv4/DHCP with no effective address.
- Effective IPv4 address.
- Default IPv4 gateway presence.
- Configured DNS servers.
- A bounded DNS test through the system resolver when an address and DNS are already present.
- NetworkManager Internet connectivity state.
- Wi-Fi radio state, adapter availability, and failed Wi-Fi activation.

Diagnostics explicitly keep hardware-driver installation out of scope. If NetworkManager does not expose a supported adapter, nm-hsp explains that state but does not install drivers or firmware.

Repairs are intentionally conservative. nm-hsp currently offers only:

- Create a new persistent automatic Ethernet profile when the adapter is managed, carrier is present, no active connection exists, and no compatible saved Ethernet profile is available.
- Enable autoconnect on an existing Ethernet profile.
- Activate an existing compatible Ethernet profile, including a retry of an automatic/DHCP profile that is active without an IPv4 address.
- Correct a stale Ethernet `interface-name` binding only when the saved profile's hardware-address binding matches the current adapter.
- Turn the NetworkManager Wi-Fi radio on.

Each repair requires two steps: Enter opens a review screen, then the user explicitly confirms. The review shows the adapter/profile when applicable and the exact **Before → After** change. The backend re-checks current NetworkManager state again before writing, so a stale diagnostics screen cannot blindly apply an old repair decision.

nm-hsp deliberately does **not** guess or automatically rewrite:

- A different saved MAC address.
- Gateway addresses.
- DNS server addresses.
- Static IP configuration.
- NetworkManager managed/unmanaged policy.
- Missing hardware drivers or firmware.

After a successful repair, the diagnostics screen immediately reloads NetworkManager state and reruns the checks.

See `docs/diagnostics.md` for the repair safety model and scenario coverage.

## Source layout

- `cmd/nm-hsp` — application entry point
- `internal/networkmanager` — NetworkManager integration
- `internal/model` — application-facing network state
- `internal/ui` — terminal user interface
- `internal/security` — secret handling and safety helpers
- `internal/validation` — network input validation
- `internal/diagnostics` — diagnostic decision engine and bounded active probes
- `docs/security.md` — credential-handling guarantees, limitations, and regression contracts
- `docs/diagnostics.md` — diagnostics chain, repair boundaries, and safety model

## Build

Requires Go 1.25 or newer.

Run the standard Go validation locally with `go fmt`, `go vet`, `go test`, and `go build`.

## License

Apache License 2.0. See `LICENSE`.
