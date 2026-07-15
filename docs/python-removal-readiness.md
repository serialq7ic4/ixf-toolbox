# Python Removal Readiness

This report decides whether the repository can remove the Python runtime code in
the next release.

## Current Decision

Status: Ready for Python implementation deletion.

Go owns every documented CLI command family, the installed skills call the Go
`ixf` binary, release artifacts are Go binaries plus checksums only, and the test
suite no longer imports `ixf_toolbox` runtime modules. The remaining Python tree
is migration-only source that can be removed in the next staged release.

The next release deletes the Python implementation.

## Deletion Gates

| Gate | Status | Evidence |
|---|---|---|
| Go owns every documented CLI command family | Pass | `docs/go-python-parity.md` marks each documented command family as Go-owned. |
| Installed skills call Go `ixf` | Pass | `ixf setup skills` installs wrappers around the local `ixf` command. |
| Release artifacts include supported Go binaries | Pass | The release workflow builds macOS, Windows, and Linux Go binaries plus checksums. |
| Tests no longer require Python runtime implementations | Pass | `scripts/audit_python_runtime_imports.py` reports a 0-file runtime import baseline. |
| Rollback no longer needs in-repo Python implementation | Pass | GitHub Releases no longer publish Python wheel or sdist artifacts; rollback can use earlier tags if needed. |
| New-install docs avoid Python as the default | Pass | README files direct new users to the Go binary first. |
| Remaining Python package API users have a migration decision | Pass | `docs/python-api-sunset.md` says the Python package API is temporary and unsupported for new runtime work. |
| Destructive removal stage is reached | Ready | The next release is the dedicated Python implementation deletion release. |

## Current Blockers

No technical blockers remain for removing the Python runtime implementation.

Deletion is still staged rather than bundled into this release so the repository
keeps one non-destructive rollback point between readiness and removal.

## Files Covered By The Removal Release

The next removal release needs to review and update these areas:

- `src/ixf_toolbox/` Python package modules.
- Python package metadata and build settings in `pyproject.toml`.
- Python-specific tests under `tests/`.
- CI steps that install, compile, or validate the Python source tree.
- README, migration, platform, privacy, and release documentation that mention
  the temporary Python migration surface.
- Any skill wrapper text that still references Python fallback behavior.

## Removal Direction

Do not add new Python runtime work.

The next safe step is the destructive removal release: delete Python package
source, package metadata, and Python-source validation after replacing remaining
packaging contracts with Go-only checks.
