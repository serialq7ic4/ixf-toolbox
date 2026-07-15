# Go / Python Runtime Parity

This document records the current runtime ownership boundary for `ixf-toolbox`.
Go owns the installed CLI runtime, while Python stays in the repository only as
a temporary migration surface until the deletion readiness report proves it can
be removed.

## Go-owned Runtime

The GitHub Release Go binary is the default runtime for new installs and for the
agent skills installed by `ixf setup skills`.

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

## Temporary Python Migration Surface

Python remains in the tree for these reasons:

- Some Python package API callers may still import `ixf_toolbox` modules directly.
- GitHub Releases no longer publish Python wheel or sdist artifacts.
- The pytest suite still uses Python fixtures and Python package tests to guard
  the public packaging contract.
- Some remote API behavior is still covered by Python tests while those fixtures
  are ported to Go.

Python is no longer the recommended runtime for new agent installs. New users
should install the Go binary and then run `ixf setup skills`.

## Deletion Gates

Python code can only be considered for deletion after all gates are true:

- Go owns every documented CLI command family and every installed skill calls Go.
- Fixture parity covers document read/publish, OKR read/write, cookie export,
  diagnostics, setup, and update flows.
- No user-facing docs recommend Python for new installs.
- CI and release workflows publish supported Go binaries and do not require the
  Python runtime implementation for CLI behavior.
- Remaining Python package API users either have a migration path or are
  explicitly treated as unsupported in the removal release.
- A dedicated Python removal readiness report has passed local verification,
  GitHub CI, and release-asset checks for a Go-only repository.

## Known Blockers

- `docs/python-removal-readiness.md` exists, and its current decision is
  `Status: Not ready for deletion`.
- Python source still remains in the repository for migration-only tests.
- Python package API deletion is not complete.
- The test harness still imports Python modules for packaging, fixture, and
  reference-contract coverage; `tests/python_runtime_imports_allowlist.txt`
  tracks the current 8-file baseline.
- The destructive removal stage has not been reached because technical deletion
  gates remain blocked.

Until these blockers are resolved, keep Python in the repository.
