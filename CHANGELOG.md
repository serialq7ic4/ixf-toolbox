# Changelog

## Unreleased

- Added the initial Go POC command surface for version, diagnostics, skill setup, and safe cookie-export failure behavior.
- Added Go-native local document commands for `docs read`, `docs outline`, `docs chunk`, `docs inspect`, and `docs cleanup`.
- Added Go-native remote docx `client_vars` reads, image asset downloads, pagination merge, and basic docx block-to-Markdown parity tests.
- Added Go `update check`, checksum-verified `update self --apply`, and `update skills` support.
- Added release workflow generation for cross-platform Go binary artifacts and checksums.
- Kept Python as the v1.x reference runtime for cookie export, full wiki/mindnote/sheet remote parity, docs publish, and OKR write parity.

## 1.2.0 - 2026-07-14

- Added `using-ixf-toolbox` as a lightweight routing skill for document and OKR workflows.
- Expanded the default README into a full project landing page modeled after the archived reader documentation.

## 1.1.0 - 2026-07-13

- Added GitHub CI and tag release workflows.
- Added smoke and changelog extraction scripts for release validation.
- Added security, privacy, contribution, platform, release, issue, and PR documentation.
- Added engineering asset contract tests to keep public project scaffolding in place.
- Added a legacy reader/writer migration guide with explicit command and skill mapping.
- Added migrated reader image asset, remote client-vars, and Windows cookie provider tests.
- Added migrated docx conversion and Markdown chunking contract tests.
- Added `cryptography` to the dev extra so AES cookie contracts run in normal test environments.

## 1.0.0 - 2026-07-13

- Marked `ixf-toolbox` as the first stable Toolbox-native release.
- Stabilized the public `ixf` command surface for docs, OKR, cookies, doctor, setup, and update workflows.
- Confirmed legacy reader/writer packages are no longer runtime dependencies.
- Updated install documentation to target the `v1.0.0` release wheel.
- Aligned reader skill cookie refresh and OKR read examples with the stable `ixf` CLI.

## 0.10.0 - 2026-07-13

- Added friendly `ixf --help`, `ixf docs --help`, and `ixf okr --help` command listings.
- Changed missing `docs` and `okr` subcommands to print available subcommands while preserving usage-error exit codes.
- Added CLI help contract tests for the most common discovery paths.

## 0.9.0 - 2026-07-13

- Added `ixf update self` for one-command Toolbox package upgrades plus skill refresh.
- Kept self-update dry-run by default; real package changes require `--apply`.
- Changed self-update execution to use safe argument vectors instead of shell command execution.

## 0.8.0 - 2026-07-13

- Removed legacy reader/writer runtime dependencies from the Toolbox package.
- Changed `ixf doctor` to report native Toolbox capabilities instead of legacy engine status.
- Removed the unused command delegation bridge now that core document, OKR, cookie, and diagnostics flows are native.

## 0.7.0 - 2026-07-13

- Added Toolbox-owned document publisher core for API-only Markdown-to-docx publishing.
- Changed `ixf docs publish` to run natively instead of delegating to `ixfwrite`.
- Preserved dry-run-by-default behavior, Markdown block conversion, document creation, content write, and required-text verification.

## 0.6.0 - 2026-07-13

- Added Toolbox-owned OKR writer core for API-only Objective and KR writes.
- Changed `ixf okr write` to run natively instead of delegating to `ixfwrite`.
- Preserved dry-run-by-default behavior, targeted `--objective-index` writes, non-target Objective protection, and publish-after-edit semantics.

## 0.5.0 - 2026-07-13

- Added Toolbox-owned OKR reader core for OKR detail API access, markdown rendering, and safe error reporting.
- Changed `ixf okr read` to run natively instead of delegating to `ixfdoc`.
- Kept `ixf okr write` on the existing compatibility path while OKR write migration remains separate.

## 0.4.0 - 2026-07-13

- Moved document reading, source inspection, Markdown chunking, artifact writing, and cleanup into Toolbox-owned docs core modules.
- Changed `ixf docs read`, `ixf docs outline`, `ixf docs chunk`, `ixf docs inspect`, and `ixf docs cleanup` to run natively instead of delegating to `ixfdoc`.
- Kept document publish and OKR workflows on the existing compatibility path while docs read migration continues.

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
