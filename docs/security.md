# Security model

NetworkManager-HSP is designed to configure ordinary Ethernet and Wi-Fi networking without exposing credentials through shell commands, process arguments, logs, or saved application state.

## Secret-handling guarantees

- NetworkManager is accessed directly over the system D-Bus. nm-hsp does not invoke `nmcli`, shells, or external processes for credential-bearing operations.
- Saved Wi-Fi passwords are not fetched into the application. The saved-profile UI displays `Password: unchanged` and the production source contract rejects use of NetworkManager `GetSecrets`.
- New Wi-Fi passwords are masked by default. They are visible only when the user explicitly enables Show Password.
- A new password is copied into `security.Secret`, whose formatting methods always render `[REDACTED]`.
- Copies of a `Secret` share one mutable state. Clearing any copy overwrites the mutable backing bytes and makes all copies read empty.
- The visible password field is cleared immediately when Connect is submitted, not after the network operation finishes. Cancellation clears it as well.
- The UI owns a second cleanup boundary around the asynchronous connection command and clears its request secret when the command completes.
- The NetworkManager backend owns an additional cleanup boundary and clears its request copy on return.
- D-Bus connection errors are terminally redacted before they are returned to the UI. The sanitized error intentionally does not wrap the original error.
- The temporary NetworkManager settings map removes the `psk` entry immediately after the D-Bus call.
- nm-hsp has no application credential cache, secret configuration file, or secret persistence layer.

## Source contracts

Automated tests inspect production Go source and fail if it introduces:

- `os/exec` process execution.
- standard `log` or `log/slog` logging.
- NetworkManager `GetSecrets` calls or literals.

Additional tests cover masked rendering, Show Password behavior, form clearing on submit, shared secret clearing, redacted formatting, D-Bus error redaction, and removal of the temporary settings-map PSK.

These checks are deliberate guardrails. Adding one of the forbidden mechanisms later should require an explicit security review rather than silently changing the credential model.

## Important Go/D-Bus limitation

NetworkManager's D-Bus API expects the WPA/WPA2/WPA3 secret as a string value inside the connection settings object.

Go strings are immutable and the runtime does not provide a reliable way to securely erase every temporary string allocation or copy. nm-hsp therefore does **not** claim cryptographic secure erasure of every in-process copy.

Instead, the application minimizes exposure by:

- keeping the primary secret in mutable bytes,
- shortening its lifetime,
- sharing clear-state across request copies,
- creating a plain string only at the D-Bus boundary,
- removing the settings-map reference immediately after the D-Bus call,
- redacting every application-controlled error/formatting path,
- and avoiding shell arguments, logs, caches, and secret retrieval.

This is the practical security boundary for the current Go/D-Bus implementation.

## Saved profiles

Existing NetworkManager profiles remain owned by NetworkManager. nm-hsp reads normal non-secret profile metadata using `GetSettings` and lets NetworkManager use its existing credentials when activating saved profiles.

Editing autoconnect or priority does not request or display the saved password.

## Current scope

New Open, OWE, WPA/WPA2 Personal, and WPA3 Personal connections are supported.

New Enterprise/802.1X and legacy WEP credential entry remain intentionally unsupported. Existing saved Enterprise profiles can be activated because their credentials stay with NetworkManager.
