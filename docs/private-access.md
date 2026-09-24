# Private Access

NetworkManager-HSP can integrate with Tailscale and NetBird when those providers are already installed on the host.

nm-hsp does not install either provider.

## What nm-hsp shows

For supported providers, the Private Access screen can show:

- whether the provider is installed
- whether its service is enabled
- whether its service is running
- connection state
- current VPN addresses when available

## Available controls

nm-hsp can:

- start an inactive provider service
- connect or authenticate
- disconnect while leaving the service running
- stop the service for the current boot

The application does not change the provider's boot policy.

The operating system or product image remains responsible for whether `tailscaled.service` or `netbird.service` is enabled at boot.

## Permissions

Packaged deployments can use the narrow Polkit rule in:

`contrib/polkit/49-nm-hsp-vpn.rules`

That rule is intended to let trusted users start and stop only the supported Tailscale and NetBird services without granting generic systemd control.

## Provider ownership boundary

Tailscale and NetBird keep ownership of their own configuration, authentication, and persistent state.

nm-hsp acts as a local control surface for supported lifecycle operations and status display. It does not replace the provider CLI, rewrite provider configuration, or manage provider installation.
