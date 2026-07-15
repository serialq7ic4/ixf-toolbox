# Python Removal Readiness

This report decides whether the repository can remove the Python runtime code in
the current release.

## Current Decision

Status: Not ready for deletion.

Go owns every documented CLI command family, and the Go binary is the default
runtime for new agent installs. Python still remains in this release because the
repository intentionally publishes Python artifacts, documents the Python
package API as legacy/reference in `docs/python-api-sunset.md`, and uses Python
modules in the test and packaging contracts.

Keep Python in this release.

## Deletion Gates

| Gate | Status | Evidence |
|---|---|---|
| Go owns every documented CLI command family | Pass | `docs/go-python-parity.md` marks each documented command family as Go-owned. |
| Installed skills call Go `ixf` | Pass | `ixf setup skills` installs wrappers around the local `ixf` command. |
| Release artifacts include supported Go binaries | Pass | The release workflow builds macOS, Windows, and Linux Go binaries plus checksums. |
| Tests no longer require Python runtime implementations | Blocked | Tests still import Python modules for package, fixture, and reference-contract coverage. |
| Rollback no longer needs in-repo Python implementation | Blocked | Python wheel and sdist are still published as legacy/reference rollback artifacts. |
| New-install docs avoid Python as the default | Pass | README files direct new users to the Go binary first. |
| Remaining Python package API users have a migration decision | Blocked | `docs/python-api-sunset.md` documents the Python package API as legacy/reference, but it is not removed. |
| Destructive removal stage is reached | Blocked | Technical deletion gates are still blocked in this release. |

## Current Blockers

- Python wheel and sdist are still part of the release artifact contract.
- Python package API compatibility is still documented as legacy/reference in
  `docs/python-api-sunset.md`.
- The test harness still imports `ixf_toolbox` modules for packaging, fixtures,
  and reference-contract coverage.
- CI and release workflows still validate Python packaging.
- Python deletion must wait for the staged removal release after technical gates
  are cleared.

## Files Covered By A Future Removal

A later approved removal would need to review these areas:

- `src/ixf_toolbox/` Python package modules.
- Python package metadata and build settings in `pyproject.toml`.
- Python-specific tests under `tests/`.
- CI and release workflow steps that build or install Python wheel/sdist
  artifacts.
- README, migration, platform, privacy, and release documentation that mention
  Python legacy/reference support.
- Any skill wrapper text that still references Python fallback behavior.

## Removal Direction

Do not delete Python code in this release.

The next safe step is to remove Python wheel and source distribution artifacts
from GitHub Releases in a separate staged release, while keeping the Python
source tree until test and API blockers are cleared.
