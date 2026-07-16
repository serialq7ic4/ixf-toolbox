# Messenger Service Design

## Goal

Add i讯飞 Messenger as a first-class `ixf-toolbox` domain while preserving the project rule that all supported runtime behavior is owned by the Go `ixf` binary.

## Scope

The first production stage is `v3.3.0` and deliberately ships foundations only:

- `ixf messenger doctor` for secret-safe local readiness checks.
- `ixf messenger open --dry-run` for validated target/open planning.
- Browser/profile discovery for macOS and Windows.
- Safe profile cloning that never opens the live LarkShell profile directly.
- Agent skill routing for messenger read/write workflows.

Real unread extraction and real message sending are later stages. Sending will stay unavailable until the Go implementation can verify the opened target, send only under `--apply`, and re-open a fresh session to confirm the sent message is present.

## Architecture

Messenger sits beside `docs` and `okr` as `ixf messenger ...`. The Go CLI delegates platform-specific readiness and profile operations to `internal/messenger`.

The browser model is profile-first, not cookie-only. The implementation clones the local LarkShell Chromium profile into a temporary directory, removes singleton locks and cache-heavy directories, and optionally injects exported cookies. This keeps the live desktop profile unlocked and avoids polluting the user profile during automation.

## Platform Rules

- macOS is Tier 1 and discovers `profile_explorer` under `~/Library/Application Support/LarkShell-ka-kaahyz17/aha/users/*/profile_explorer`.
- Windows is supported experimentally through the LarkShell Chromium profile under `%APPDATA%\LarkShell\User Data\Default`.
- Linux is unsupported for messenger because there is no local i讯飞 desktop client profile to reuse.

## Safety

- Diagnostics must never print cookie values, CSRF tokens, message bodies, private IDs, or raw profile content.
- `open` requires an explicit target and `--mode person|conversation`.
- `open` is dry-run-only in `v3.3.0`; it reports what would be opened and whether local prerequisites are present.
- Real write operations must not exist until target verification and fresh-session verification are implemented.

## Testing

The first stage is covered by Go tests for CLI command contracts, platform support, profile discovery, clone cleanup, secret-safe diagnostics, and skill installation.
