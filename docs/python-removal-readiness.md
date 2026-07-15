# Python Removal Readiness

This report records the deletion decision and final state for the Python runtime
implementation.

## Current Decision

Status: Python implementation deleted.

Go owns every documented CLI command family, the installed skills call the Go
`ixf` binary, release artifacts are Go binaries plus checksums only, and the test
suite no longer imports `ixf_toolbox` runtime modules. The repository no longer
contains the Python runtime/package implementation.

## Deletion Gates

| Gate | Status | Evidence |
|---|---|---|
| Go owns every documented CLI command family | Pass | `docs/go-python-parity.md` marks each documented command family as Go-owned. |
| Installed skills call Go `ixf` | Pass | `ixf setup skills` installs wrappers around the local `ixf` command. |
| Release artifacts include supported Go binaries | Pass | The release workflow builds macOS, Windows, and Linux Go binaries plus checksums. |
| Tests no longer require Python runtime implementations | Pass | `scripts/audit_python_runtime_imports.py` reports a 0-file runtime import baseline. |
| Rollback no longer needs in-repo Python implementation | Pass | GitHub Releases no longer publish Python wheel or sdist artifacts; rollback can use earlier tags if needed. |
| New-install docs avoid Python as the default | Pass | README files direct new users to the Go binary first. |
| Remaining Python package API users have a migration decision | Pass | `docs/python-api-sunset.md` says the Python package API has been removed. |
| Destructive removal stage is reached | Pass | `src/ixf_toolbox/` and Python package metadata have been removed. |

## Current Blockers

No technical blockers remain. The Python runtime/package implementation has
already been removed.

## Removed Areas

The removal release deleted or replaced these areas:

- `src/ixf_toolbox/` Python package modules.
- Python package metadata and build settings in `pyproject.toml`.
- Python runtime tests under `tests/`.
- CI steps that installed, compiled, or validated the Python source tree.
- Python wheel smoke flow in `scripts/smoke.sh`.

## Final State

Do not add new Python runtime work.

Python remains only as a repository test harness for pytest/ruff and helper
scripts. The supported user runtime is the Go `ixf` binary.
