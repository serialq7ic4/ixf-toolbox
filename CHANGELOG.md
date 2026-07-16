# Changelog

## Unreleased

## 3.1.0 - 2026-07-16

- Removed the Python pytest harness and repository maintenance Python scripts; `go test ./...` is now the primary project test entrypoint.
- Added Go CLI integration tests for document publish, document read cleanup, OKR write apply flows, and self-update release artifacts.
- Updated CI and Release workflows to use the Go toolchain only.
- Updated README agent usage examples so users describe the task naturally while `using-ixf-toolbox` handles routing in the background.

## 3.0.0 - 2026-07-16

- Deleted the Python runtime/package implementation and removed the Python wheel smoke path.
- Converted `pyproject.toml` to test-tool configuration only; release versioning is now owned by `VERSION` and the Go CLI.
- Updated CI and Release workflows to install only pytest/ruff test tools while building and publishing Go binary artifacts.
- Updated documentation to describe v3.0 as a Go-only runtime repository.

## 2.18.0 - 2026-07-16

- Marked Python removal readiness as ready after Go-owned CLI parity, Go-only release assets, and a 0-file Python runtime import baseline.
- Documented that the next release deletes the Python implementation rather than adding more migration-only Python work.
- Updated install examples and version metadata for the final non-destructive readiness release.

## 2.17.0 - 2026-07-16

- Replaced residual Go POC Python reference imports with static Go-owned golden assertions.
- Removed all remaining pytest imports of `ixf_toolbox` runtime modules.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 1 file to 0 files.
- Updated Python removal readiness docs to point the next stage at destructive Python package removal.

## 2.16.0 - 2026-07-16

- Added Go OKR reader tests for URL detection, LGW CSRF refresh, `okr_id` detail requests, exact Markdown rendering, and private-payload-safe API errors.
- Removed the OKR Python runtime test after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 2 files to 1 file.
- Updated Python removal readiness docs to point the next migration stage at residual Go POC Python reference imports.

## 2.15.0 - 2026-07-16

- Added Go remote docs reader coverage for non-docx mindnote reads with image download enabled while keeping artifact collectors empty.
- Removed the remote docs reader Python runtime test after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 3 files to 2 files.
- Updated Python removal readiness docs to point the next migration stage at OKR coverage and residual Python reference imports.

## 2.14.0 - 2026-07-16

- Added Go docx converter tests for resolver panic redaction, nested resolver token leaks, ordered lists, and nested bullet/callout rendering.
- Hardened image resolver token scanning for typed nested string maps and string slices.
- Removed the document converter Python runtime test after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 4 files to 3 files.

## 2.13.0 - 2026-07-16

- Added Go image asset writer tests for safe downloads, deduplication, caption preservation, sanitized failure warnings, fallback image magic validation, and stale generated-file cleanup.
- Added one-time stale generated-image cleanup before downloads without deleting non-generated files in the asset directory.
- Removed the document image asset Python runtime test after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 5 files to 4 files.

## 2.12.0 - 2026-07-16

- Added Go local docs tests for local Markdown reads, output file-stem collision handling, manifest writes, generated-output cleanup, and inspect-source redaction.
- Added Go Markdown chunking tests for multiple H1 section selection and oversized H2 sections that split at H3 breadcrumbs.
- Aligned Go `docs read` manifest JSON output with the migrated Python contract by removing the trailing newline.
- Removed local docs and Markdown chunking Python runtime tests after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 7 files to 5 files.

## 2.11.0 - 2026-07-16

- Added Go Windows cookie provider tests for cookie DB and Local State discovery, DPAPI master-key unwrapping, AES-GCM cookie decrypt, legacy DPAPI cookie decrypt, and Windows export fixtures.
- Added a testable DPAPI seam and normalized Windows decrypt errors without changing production behavior.
- Removed the Windows cookie provider Python runtime test after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 8 files to 7 files.

## 2.10.0 - 2026-07-15

- Added Go cookie core tests for private cookie JSON writes, macOS cookie DB discovery, macOS plain-row export, and keychain command argument safety.
- Added a testable keychain command seam while keeping production cookie export behavior unchanged.
- Removed cookie core and CLI delegation Python runtime tests after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 10 files to 8 files.

## 2.9.0 - 2026-07-15

- Added Go setup and doctor contract tests covering runtime normalization, skill installation, secret-safe diagnostics, and doctor text/JSON output.
- Removed setup and doctor Python runtime tests after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 13 files to 10 files.
- Clarified current documentation that Python is a temporary migration surface on the path to deletion, not a long-term legacy/reference runtime.

## 2.8.0 - 2026-07-15

- Added Go tests for CLI version/help contracts and update version/release checks.
- Removed local CLI/update Python runtime tests from the deletion allowlist after equivalent Go coverage was in place.
- Reduced `tests/python_runtime_imports_allowlist.txt` from 17 files to 13 files.

## 2.7.0 - 2026-07-15

- Added a Python runtime import audit script to keep the remaining Python test dependency list explicit.
- Added `tests/python_runtime_imports_allowlist.txt` as the baseline for Python runtime imports that must shrink before deletion.
- Updated Python removal readiness documentation to point the next stage at reducing the import allowlist.

## 2.6.0 - 2026-07-15

- Changed GitHub Releases to publish Go-only release artifacts: platform binaries plus checksums.
- Stopped publishing Python wheel/sdist artifacts while keeping Python source temporarily for reference tests.
- Changed release tag validation to use the runtime-neutral `VERSION` file instead of Python package metadata.
- Updated README and release documentation to remove release-wheel install and smoke expectations.

## 2.5.1 - 2026-07-15

- Added a runtime-neutral `VERSION` file so future Go-only release stages do not depend on Python package metadata as the only version source.
- Added a Python API sunset policy documenting the Python package API as legacy/reference with no new Python runtime features.
- Updated install documentation to point at the v2.5.1 release artifacts while keeping Python deletion blocked by technical gates only.
- Corrected the Go `update self --apply` release fixture to test an upgrade from the current version to the next version.

## 2.5.0 - 2026-07-15

- Added a runtime-neutral `VERSION` file so future Go-only release stages do not depend on Python package metadata as the only version source.
- Added a Python API sunset policy documenting the Python package API as legacy/reference with no new Python runtime features.
- Updated install documentation to point at the v2.5.0 release artifacts while keeping Python deletion blocked.

## 2.4.0 - 2026-07-15

- Added a Python removal readiness report that keeps Python in the release until deletion blockers are resolved.
- Documented remaining blockers around Python wheel/sdist artifacts, Python package API support, and explicit deletion approval.
- Updated install documentation to point at the v2.4.0 release artifacts.

## 2.3.0 - 2026-07-15

- Added a Go/Python parity matrix documenting Go-owned CLI runtime behavior and Python legacy/reference scope.
- Documented Python deletion gates and known blockers before any Python removal can be considered.
- Updated install documentation to point at the v2.3.0 release artifacts.

## 2.2.0 - 2026-07-15

- Added Go OKR full-spec OKR writes with `write --apply` support without `--objective-index`, matching Objectives by text.
- Added Go OKR explicit prune support with `--prune` for deleting non-input Objectives and replacing target KRs.
- Preserved non-prune OKR writes by appending/reordering requested KRs while keeping extra KRs and Objectives.

## 2.1.0 - 2026-07-15

- Added Go OKR `write --apply --objective-index` support for creating the next Objective when the target index is exactly one past the current Objective count.
- Added Go OKR draft-version retry handling for stale draft responses during Objective/KR writes.
- Added a staged Go full-replacement plan that keeps Python until explicit deletion gates are met.

## 2.0.0 - 2026-07-15

- Made the GitHub Release Go binary the default install path and default local runtime for new installs.
- Documented that the Go binary is the default install path for Codex and Claude Code skill setup.
- Documented that the Python wheel remains legacy/reference for rollback, parity checks, and Python package API callers.
- Updated README, migration, and platform docs for Go-first installation while keeping dry-run-first write safety.
- Changed Go `doctor --json` runtime reporting from `go-poc` to `go`.

## 1.8.0 - 2026-07-15

- Hardened Go CLI cookie export help so `ixf cookies export --help` exits successfully and lists provider-specific options on stdout.
- Added OKR write apply gating coverage to prove `--objective-index` validation happens before cookie loading.
- Updated release-note contract coverage for the v1.8 release boundary and advanced self-update fixtures toward `v2.0.0`.

## 1.7.0 - 2026-07-15

- Added Go-native cookie export for macOS and Windows LarkShell Chromium profiles with local SQLite cookie DB reads.
- Added macOS Keychain AES-CBC and Windows DPAPI / AES-GCM decryption seams while keeping fixture coverage secret-safe.
- Added Go CLI cookie export flags for explicit cookie DB, host filters, app support paths, app data, local state, and keychain selectors.
- Updated Go diagnostics so `doctor --json` reports `cookiesExport=true`, and added `go.sum` to pin the SQLite dependency for release builds.

## 1.6.0 - 2026-07-15

- Added Go-native OKR `write --apply --objective-index` for replacing one selected Objective and its KRs through the API.
- Entered the published Objective edit/draft state before mutation, then re-published the Objective after replacement.
- Added fixture-backed coverage for preserving non-target Objectives, deleting old KRs, creating replacement KRs, and verifying post-publish content.
- Kept Go OKR write scoped to explicit Objective-index writes while broader OKR mutation flows continue to use the Python reference runtime.

## 1.5.0 - 2026-07-15

- Added Go-native `okr read` routing with authorized OKR detail API reads, local session cookies, LGW CSRF refresh, and Markdown rendering.
- Added Go-native `okr write` dry-run planning for approved Objective / KR JSON input without requiring cookies or remote mutation.
- Added fixture-backed Go CLI coverage for OKR help, OKR read CSRF/session headers, Markdown rendering, and OKR write dry-run validation.
- Kept real OKR write `--apply` on the Python reference runtime while the Go API mutation flow is migrated in a later release.

## 1.4.0 - 2026-07-15

- Added Go-native `docs publish --apply` API-only document creation, content write, and verification using cookie-gated apply semantics.
- Added fixture-backed Go CLI coverage for create, `client_vars`, `user_change`, required-text verification, multiline code preservation, CSRF headers, and session cookies.
- Updated Go migration documentation to mark docs publishing apply support as migrated while keeping cookie export and OKR write on the Python reference runtime.

## 1.3.0 - 2026-07-15

- Added the initial Go POC command surface for version, diagnostics, skill setup, and safe cookie-export failure behavior.
- Added Go-native local document commands for `docs read`, `docs outline`, `docs chunk`, `docs inspect`, and `docs cleanup`.
- Added Go-native remote docx `client_vars` reads, image asset downloads, embedded sheet expansion, pagination merge, and basic docx block-to-Markdown parity tests.
- Added Go-native wiki links that resolve through page HTML to docx tokens, then reuse the remote docx reader.
- Added Go-native wiki bitable reads from `clientvars` gzip schema with TSV manifest output.
- Added Go-native direct mindnote reads from page `clientVars` HTML with Markdown tree rendering.
- Added Go/Python golden parity coverage for mixed remote docx blocks and aligned Go image content validation for SVG, BMP, and TIFF downloads.
- Added Go-native `docs publish` dry-run planning for Markdown titles and block counts without cookie loading.
- Clarified Go `docs read` routing so OKR page URLs fail before cookie loading with an `ixf okr read` hint.
- Refactored Go CLI contract fixtures to share remote docx server, cookie, and sheet payload helpers for the remaining v1.4 parity work.
- Added Go `update check`, checksum-verified `update self --apply`, and `update skills` support.
- Added release workflow generation for cross-platform Go binary artifacts and checksums.
- Kept Python as the v1.x reference runtime for cookie export, remaining wiki edge-variant remote parity, docs publish apply, and OKR write parity.

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
