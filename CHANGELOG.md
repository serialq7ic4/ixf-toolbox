# Changelog

## Unreleased

## 3.13.1 - 2026-07-21

- Fixed source-built `ixf --version` so it reads the repository `VERSION` value by default instead of drifting behind release builds.
- Added regression coverage that fails when the default CLI version and `VERSION` file diverge.

## 3.13.0 - 2026-07-20

- Added a dedicated `ixf sheets` command surface with direct sheet reads and TSV update dry-runs.
- Added `sheetsRead` and `sheetsUpdateDryRun` doctor capabilities so agents can discover sheet support without relying on historical docs.
- Updated README, routing docs, and agent skills to route sheet cell work through `ixf sheets update --dry-run` and to treat `sheets update --apply` as unavailable until a real write API contract is captured.

## 3.12.0 - 2026-07-20

- Added docs publish/update table fallback metadata so dry-runs report when Markdown tables will be preserved as readable callout fallback blocks.
- Hardened docs publish/update verification with `missingRequiredText` and `emptyCalloutCount` diagnostics, and made empty callout regressions fail verification.
- Updated docs writer skills and docs update guidance so agents inspect structural verification fields before claiming write success.

## 3.11.3 - 2026-07-20

- Fixed leaf command help so `--help`, `-h`, and `help` return exit `0` on stdout for docs, OKR, Messenger, and update subcommands.
- Preserved Markdown table content in docs publish/update by parsing table rows as `table` specs and emitting readable callout fallback blocks instead of empty callouts.
- Added regression coverage for agent-discovered CLI help and Markdown table dry-run/apply payload preservation.

## 3.11.2 - 2026-07-20

- Fixed `ixf docs update --apply` for existing docx body replacement by updating root children without hard-deleting old block objects, matching the accepted `user_change` contract.
- Fixed docx writer attributed-text lengths for Chinese, emoji, and other non-ASCII content by counting UTF-16 code units instead of Go string bytes.
- Improved docs update write failures so server rejection codes and messages are surfaced in CLI errors.

## 3.11.1 - 2026-07-20

- Fixed `ixf update self` release discovery to use GitHub Release redirects and deterministic Go artifact URLs instead of the unauthenticated GitHub REST API, avoiding shared-proxy API rate limits.

## 3.11.0 - 2026-07-20

- Added explicit `--allow-complex-replace` support for confirmed `ixf docs update --apply` operations that intentionally replace complex existing body blocks.
- Added complex-block risk metadata to docs update dry-run and apply JSON payloads.
- Added a docs update runbook and clarified Go runtime parity/release documentation for create-only publish versus existing-docx update.

## 3.10.0 - 2026-07-20

- Added `ixf docs update <file.md> --url <docx-url> --apply` for API-only existing-docx `replace_body` writes.
- The apply flow reads target state, rejects complex existing content by default, replaces root body children through `user_change`, and re-reads the document for required-text verification.
- Kept `ixf docs publish` create-only so new-document publishing and existing-document updates remain separate operations.

## 3.9.0 - 2026-07-20

- Added `ixf docs update <file.md> --url <docx-url> --dry-run` for API-only existing-docx update preflight.
- The update preflight reads authorized target document state, reports `operation:update_docx`, `mode:replace_body`, destructive status, current and planned top-level block counts, and complex-block risk.
- Kept `docs update --apply` disabled until the apply release so no existing docx content can be mutated in v3.9.0.

## 3.8.1 - 2026-07-20

- Clarified that `ixf docs publish` is create-only and does not overwrite existing docx documents.
- Added `operation:create_docx` to docs publish dry-run and apply JSON payloads.
- Hardened docs writer and routing skill guidance to avoid claiming existing-document update support before `ixf docs update` ships.

## 3.8.0 - 2026-07-20

- Added `ixf doctor --json` agent routing diagnostics so agents can verify Go-only background routing and ignore historical implementation notes.
- Added `docs/agent-routing.md` and hardened `using-ixf-toolbox` skill guidance for natural user prompts and read-only default routing.
- Added Messenger stability metadata to `ixf messenger doctor --json`, documenting local-browser automation, platform support levels, and send success criteria.

## 3.7.2 - 2026-07-17

- Fixed Windows CI coverage for Go-only legacy command diagnostics by creating platform-appropriate `ixfdoc` and `ixfwrite` shim fixtures.
- Kept legacy command reporting as ignored diagnostics only; the runtime remains Go `ixf` only.

## 3.7.1 - 2026-07-17

- Added Go-only agent guidance to prevent Python fallback or legacy `ixfdoc`/`ixfwrite` routing when agents inspect historical project notes.
- Updated all installed ixf skill wrappers with an explicit runtime boundary requiring Go `ixf` only.
- Added `ixf doctor` legacy command detection that reports old shims as ignored without changing the Go-only runtime.

## 3.7.0 - 2026-07-17

- Added secret-safe Messenger doctor remediation guidance for missing platform, profile, Chrome/Chromium browser, and cookie prerequisites.
- Added a Messenger GA runbook covering Chrome/Chromium-only automation, cloned profile isolation, read-side effects, send verification criteria, and troubleshooting.
- Updated Messenger reader/writer skills and platform docs to align agent behavior with the GA safety boundaries.

## 3.6.2 - 2026-07-17

- Fixed Messenger target search by using native browser click events, CDP text insertion, and safe selector diagnostics when the global search modal does not accept input.
- Fixed approved Messenger sends for account-id recipients by allowing fresh-session verification to reopen the latest recent chat when the verified title contains the account id.
- Changed Messenger automatic browser discovery to Chrome/Chromium only; Edge is no longer selected as an automatic fallback.

## 3.6.1 - 2026-07-17

- Fixed Messenger diagnostics so `ixf messenger doctor --json` reports approved send automation as available after the `v3.6.0` send release.

## 3.6.0 - 2026-07-17

- Added Go-native Messenger `send --apply` browser automation with pre-send target verification, local echo matching, and mandatory fresh-session verification through a second cloned profile.
- Added `ixf messenger send --to <target> --mode person|conversation --message <text> --dry-run|--apply --json`, with dry-run planning that does not launch a browser and result payloads that do not echo full message bodies.
- Updated Messenger writer skills and docs to allow approved sends while preserving dry-run-first and verification-first safety boundaries.

## 3.5.0 - 2026-07-16

- Added Go-native Messenger `read --apply` browser automation for recent or unread conversation extraction through a cloned LarkShell profile.
- Added `ixf messenger read --scope unread|recent --dry-run|--apply --json`, with conversation limits, per-chat message limits, headless default behavior, and explicit visible fallback opt-in.
- Updated Messenger reader skills and docs to expose read-only extraction while keeping real sends unavailable.

## 3.4.0 - 2026-07-16

- Added Go-native Messenger `open --apply` browser automation through a cloned LarkShell profile, with target verification and no message sending.
- Added `chromedp`-based Messenger automation, cookie injection, title matching, headless default behavior, and explicit visible fallback opt-in.
- Updated Messenger skills and documentation to distinguish dry-run planning from applied open verification, and to keep real sends unavailable.

## 3.3.0 - 2026-07-16

- Added the first Go-native Messenger foundation commands: `ixf messenger doctor` and dry-run-only `ixf messenger open`.
- Added Messenger desktop profile discovery, safe profile cloning primitives, browser readiness checks, and secret-safe diagnostics.
- Added Messenger reader/writer agent skills and routed Messenger requests through `using-ixf-toolbox` while keeping real sends unavailable until verification support lands.
- Updated README and platform documentation for the staged Messenger rollout.

## 3.2.0 - 2026-07-16

- Added direct `/sheets/...` document reads through the Go `docs read` flow, including safe `inspect` routing and TSV rendering via the existing sheet `client_vars` decoder.

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
