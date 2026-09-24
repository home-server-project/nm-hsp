# Development

This document contains source-build, repository-layout, validation, and packaging details for NetworkManager-HSP.

## Source build

Go 1.25 or newer is required.

```bash
go build -trimpath -o nm-hsp ./cmd/nm-hsp
./nm-hsp
```

Ordinary local builds show `devel` as the application version.

Packaged testing and release builds inject the version automatically at build time.

## Repository layout

- `cmd/nm-hsp` — application entry point
- `internal/buildinfo` — build-time version information
- `internal/networkmanager` — NetworkManager integration
- `internal/vpn` — Tailscale, NetBird, and systemd integration
- `internal/ui` — terminal interface
- `internal/diagnostics` — health and repair logic
- `internal/security` — secret handling
- `internal/validation` — input validation

## Validation

Before changes are promoted, the normal source checks are:

```bash
gofmt -w .
go vet ./...
go test ./...
go build -trimpath -o /tmp/nm-hsp ./cmd/nm-hsp
```

The GitHub Actions source-test workflow runs the equivalent validation on repository changes.

## Build versions

Version text is not manually edited in the application source.

- Local development build: `devel`
- Testing package build: `testing-<commit>`
- Release build: the Git tag, for example `v0.3.0`

Release builds inject the tag into `internal/buildinfo.Version`.

## Packaged builds

Home Server Project images are expected to consume packaged nm-hsp builds instead of asking end users to compile the application.

Homebrew packaging is maintained in:

https://github.com/home-server-project/homebrew-tap

The tap has a testing path for prebuilt binaries before an official nm-hsp release is created. Stable packaging is intended to consume official release assets.
