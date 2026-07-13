# Changelog

## 0.3.0 - 2026-07-13

- Moved cookie/session export and diagnostics into Toolbox-owned core modules.
- Changed `ixf cookies export` to use native Toolbox cookie providers instead of delegating to the legacy writer command.
- Kept cookie diagnostics secret-safe while preserving macOS and Windows desktop session export support.

## 0.2.0 - 2026-07-13

- Added native `ixf doctor` diagnostics for Toolbox version, legacy engines, installed agent skills, and cookie metadata.
- Kept cookie diagnostics secret-safe by reporting names and boolean flags without cookie values.
- Changed `ixf doctor` from a writer-engine passthrough to a Toolbox-owned command.

## 0.1.1 - 2026-07-13

- Rewrote delegated legacy engine output so user-facing hints use `ixf` commands instead of `ixfdoc` or `ixfwrite`.
- Updated install examples to target the `v0.1.1` release wheel.

## 0.1.0 - 2026-07-13

- Added unified `ixf` CLI as a compatibility bridge over existing reader and writer engines.
- Added Codex and Claude Code skill wrappers for docs reader, docs writer, OKR reader, and OKR writer.
- Added release update check and skill refresh commands.
- Documented the staged migration path from standalone reader/writer projects to Toolbox.
