# Python Removal Readiness

This report records the deletion decision and final state for the Python runtime
implementation.

## Current Decision

Status: Python implementation deleted.

Go owns every documented CLI command family. Native Codex and Claude plugins
contain generated copies of `skills/*/SKILL.md` and resolve the Go `ixf` binary;
when the runtime is absent or too old, bootstrap occurs only after explicit user
confirmation. Release artifacts are Go binaries plus checksums only, and the test
suite is Go-only. The repository no longer contains Python runtime/package or
Python test harness source files.

## Deletion Gates

| Gate | Status | Evidence |
|---|---|---|
| Go owns every documented CLI command family | Pass | `docs/go-python-parity.md` marks each documented command family as Go-owned. |
| Installed skills call Go `ixf` | Pass | Native Codex and Claude plugin packages contain canonical Go-runtime skills and confirmed user-local bootstrap metadata. |
| Release artifacts include supported Go binaries | Pass | The release workflow builds macOS, Windows, and Linux Go binaries plus checksums. |
| Tests no longer require Python runtime implementations | Pass | `go test ./...` owns the repository test suite and no `.py` source files remain. |
| Rollback no longer needs in-repo Python implementation | Pass | GitHub Releases no longer publish Python wheel or sdist artifacts; rollback can use earlier tags if needed. |
| New-install docs avoid Python as the default | Pass | README files direct new users to native plugins backed by the Go binary. |
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
- Python pytest harness and Python repository maintenance scripts.
- CI steps that installed, compiled, or validated the Python source tree.
- Python wheel smoke flow in `scripts/smoke.sh`.

## Final State

Do not add new Python runtime work.

Do not add a Python test harness or Python maintenance scripts. The supported
user runtime and development test entrypoint are both Go-owned.

Native plugin installation is the supported skill path. Plugin bootstrap may
install or update the Go executable only after user confirmation and checksum
verification. `ixf doctor --json` is read-only, and `ixf deps install --apply`
is the only optional dependency mutation path.
