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
| `docs publish` | Owned | Publishes Markdown with dry-run-first and explicit `--apply` semantics. |
| `okr read` | Owned | Reads authorized OKR pages through the OKR detail APIs. |
| `okr write` | Owned | Writes confirmed Objective / KR JSON, including index-targeted, full-spec, and explicit prune flows. |
| `cookies export` | Owned | Exports local desktop-session cookies on macOS and CI-covered Windows providers. |
| `doctor` | Owned | Reports runtime, skill, and cookie metadata without printing cookie values. |
| `setup skills` | Owned | Installs Codex and Claude Code skill wrappers that call the local `ixf` binary. |
| `update check` | Owned | Checks the latest GitHub Release without mutating local files. |
| `update self` | Owned | Plans or applies local binary/package replacement with explicit `--apply`. |
| `update skills` | Owned | Refreshes installed local skill wrappers. |

## Python Test Harness

Python remains only as a repository development/test harness:

- Pytest drives fixture-heavy CLI contract tests.
- Ruff checks Python test and helper scripts.
- `scripts/extract_changelog.py` and `scripts/audit_python_runtime_imports.py`
  remain small repository maintenance helpers.

There is no Python package API, wheel, sdist, or Python runtime implementation.
Direct Python package API callers must migrate to the Go CLI.

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
