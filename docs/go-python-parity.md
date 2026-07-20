# Go / Python Runtime Parity

This document records the current runtime ownership boundary for `ixf-toolbox`.
The repository is now a Go-only runtime repository: Go owns the installed CLI
runtime, release artifacts, and agent skill execution path.

## Go-owned Runtime

The GitHub Release Go binary is the default and supported runtime for new
installs and for the agent skills installed by `ixf setup skills`.

| Command family | Go ownership | Notes |
|---|---|---|
| `docs read` | Owned | Reads authorized remote documents and local Markdown into local artifacts. |
| `docs publish` | Owned | Creates a new docx from Markdown with dry-run-first and explicit `--apply` semantics. |
| `docs update` | Owned | Updates an existing docx body from Markdown with dry-run-first, `replace_body`, complex-block safeguards, and explicit `--apply` semantics. |
| `sheets read` | Owned | `ixf sheets read` reads direct authorized sheets links as Markdown/TSV through the sheet client-vars API. |
| `sheets update` | Dry-run owned | `ixf sheets update --dry-run` plans TSV cell updates with target token, sheet id, range, row count, and column count; `ixf sheets update --apply` remains unavailable until the real write API contract is captured. |
| `okr read` | Owned | Reads authorized OKR pages through the OKR detail APIs. |
| `okr write` | Owned | Writes confirmed Objective / KR JSON, including index-targeted, full-spec, and explicit prune flows. |
| `cookies export` | Owned | Exports local desktop-session cookies on macOS and CI-covered Windows providers. |
| `doctor` | Owned | Reports runtime, skill, and cookie metadata without printing cookie values. |
| `setup skills` | Owned | Installs Codex and Claude Code skill wrappers that call the local `ixf` binary. |
| `update check` | Owned | Checks the latest GitHub Release without mutating local files. |
| `update self` | Owned | Plans or applies local binary/package replacement with explicit `--apply`. |
| `update skills` | Owned | Refreshes installed local skill wrappers. |

## Test Harness

The repository test harness is Go-only:

- `go test ./...` covers unit, integration, CLI contract, and repository
  contract tests.
- `go vet ./...` is the supported static analysis gate in CI and release
  workflows.
- Release note extraction uses shell tooling in GitHub Actions instead of a
  repository Python script.

There is no Python package API, wheel, sdist, or Python runtime implementation.
Direct Python package API callers must migrate to the Go CLI.

## No Legacy Fallback

All current docs, wiki, docx, sheets, OKR, cookie, setup, update, and Messenger
workflows use Go `ixf` only. Do not use Python fallback, Python-compatible
readers, Python-compatible writers, `ixfdoc`, or `ixfwrite`. Old changelog
entries and `docs/superpowers/` implementation plans are historical records, not
current routing guidance.

## Deletion Gates

Python runtime deletion is complete:

- Go owns every documented CLI command family and every installed skill calls Go.
- Fixture parity covers document read/publish, OKR read/write, cookie export,
  diagnostics, setup, and update flows.
- No user-facing docs recommend Python for new installs.
- CI and release workflows publish supported Go binaries and do not require the
  Python runtime implementation for CLI behavior.
- Python package API is removed.
- `docs/python-removal-readiness.md` records the final deletion state.

## Known Blockers

No known blockers remain for Python runtime deletion.

Future runtime work should be implemented in Go and covered by Go tests or CLI
contract tests.
