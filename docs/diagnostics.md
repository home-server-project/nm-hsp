# Diagnostics and repair model

Checkpoint 7 adds a first-party Network health & repair workflow to NetworkManager-HSP.

The goal is not to turn nm-hsp into an automatic network optimizer. The goal is to tell a normal user where the connection chain broke and to offer a repair only when the intended change is narrow and defensible.

## Diagnostic chain

The diagnostics engine consumes the normalized NetworkManager snapshot and produces deterministic user-facing checks.

For Ethernet, the chain covers:

1. Adapter exposed by NetworkManager.
2. NetworkManager managed state.
3. Physical carrier/cable state.
4. Saved Ethernet profile matching.
5. Active/inactive profile state.
6. Autoconnect state.
7. Interface-name binding.
8. Saved hardware-address binding.
9. Effective IPv4 address.
10. Automatic/DHCP profile with no IPv4 address.
11. Default IPv4 gateway presence.

For Wi-Fi, the chain covers:

1. Wi-Fi radio state.
2. NetworkManager managed state.
3. Adapter unavailable state.
4. Failed activation.
5. Connected versus not connected.

At the overall-network level, the engine also covers:

- Configured DNS servers.
- DNS lookup success/failure when an address and DNS servers already exist.
- NetworkManager connectivity: full, limited, captive portal, none, or unknown.

The DNS probe uses Go's ordinary system resolver with a short timeout and does not select or hard-code a public DNS service.

## Repair principle

A diagnostic finding and a repair are separate concepts.

Many findings intentionally have no repair. A repair is attached only when nm-hsp can describe exactly what it will change and can re-validate the required evidence immediately before applying it.

The UI therefore uses two-step confirmation:

1. Select a finding with a repair and press Enter to review.
2. Review the exact Before and After values, then explicitly confirm.

A successful repair triggers a new snapshot, optional DNS probe, and complete diagnostics rerun.

## Supported guarded repairs

### Create automatic Ethernet profile

Offered only when:

- the Ethernet adapter is managed by NetworkManager,
- physical carrier is present,
- no active connection currently exists,
- and no compatible saved Ethernet profile is currently available.

Execution re-checks those conditions.

The repair creates a new persistent profile for the selected interface with:

- IPv4 automatic,
- IPv6 automatic,
- autoconnect enabled.

The new profile is then explicitly activated. Existing profiles are not modified.

### Enable autoconnect

Offered when a matching Ethernet profile has autoconnect disabled.

Execution re-reads the current saved profile and changes only the `connection.autoconnect` field to true. The full profile is written back through NetworkManager so unrelated settings remain intact.

### Activate or retry existing profile

Offered for an existing compatible inactive Ethernet profile, and for an automatic/DHCP profile that is active but has no effective IPv4 address.

The repair does not change IP settings. It asks NetworkManager to activate the existing saved profile on the selected device.

### Fix stale interface-name binding

This is intentionally stricter than simple interface-name comparison.

The repair is offered only when:

- the saved Ethernet profile is bound to a different interface name,
- the saved profile has a hardware-address binding,
- and that saved hardware address matches the current adapter.

Execution re-reads both the profile and device and checks the MAC match again. It then changes only `connection.interface-name`.

If the hardware-address evidence is absent or does not match, nm-hsp refuses this repair.

### Turn Wi-Fi radio on

Offered when NetworkManager reports the Wi-Fi radio disabled.

The repair changes only NetworkManager's Wi-Fi enabled state.

## Diagnostic-only conditions

nm-hsp deliberately diagnoses but does not automatically repair:

- Ethernet cable disconnected.
- NetworkManager unmanaged device.
- Saved profile hardware-address mismatch.
- Missing or unsupported adapter/driver.
- IPv4 address present but no default gateway.
- DNS servers missing.
- DNS lookup failure.
- Limited or unavailable Internet connectivity.
- Static IP mistakes that require choosing a new address, gateway, or DNS server.
- Wi-Fi adapter unavailable.
- Failed Wi-Fi activation that requires credential/profile review.

These cases need user intent, physical action, upstream-network changes, or information that nm-hsp cannot safely infer.

## Driver boundary

Driver and firmware installation are outside nm-hsp's scope.

If no supported Ethernet or Wi-Fi adapter is exposed through NetworkManager, diagnostics say so directly. The application does not install packages, layer host software, fetch drivers, or attempt hardware-specific remediation.

## Stale-state protection

Repair decisions are generated from a point-in-time snapshot, but execution does not trust that snapshot blindly.

Before a repair writes or activates anything, the backend re-checks the relevant current NetworkManager state, including as applicable:

- device object validity,
- device type,
- managed state,
- interface name,
- active connection state,
- available saved profiles,
- physical carrier,
- profile type and identity,
- current interface binding,
- saved hardware address,
- current device hardware address.

If the state changed enough that the original repair is no longer unambiguous, the repair fails with a request to rerun diagnostics instead of forcing the old decision.

## Regression coverage

The diagnostics tests cover the motivating JustVoxel-style failure and adjacent cases, including:

- managed Ethernet with carrier but no persistent profile,
- autoconnect disabled,
- stale interface name with matching MAC,
- stale interface name without MAC evidence,
- mismatched saved MAC with no automatic activation/repair,
- automatic/DHCP profile with no effective IPv4 address,
- address present with no gateway,
- DNS working while Internet connectivity is unavailable,
- DNS lookup failure,
- Wi-Fi radio disabled,
- and no supported adapter with the driver-install boundary explained.

UI tests verify that:

- the dashboard opens diagnostics explicitly,
- a repair cannot execute on the first Enter,
- the confirmation view shows Before and After,
- non-repairable findings do nothing on Enter,
- and repair availability is shown clearly.
