# Python Removal Readiness

This report decides whether the repository can remove the Python runtime code in
the current release.

## Current Decision

Status: Not ready for deletion.

Go owns every documented CLI command family, and the Go binary is the default
runtime for new agent installs. Python still remains in this release because the
repository intentionally publishes Python artifacts, documents the Python
package API as legacy/reference, and uses Python modules in the test and
packaging contracts.

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
| Remaining Python package API users have a migration decision | Blocked | The Python package API remains documented as legacy/reference, not removed. |
| Deletion has explicit user approval | Blocked | No explicit user approval for deletion has been given. |

## Current Blockers

- Python wheel and sdist are still part of the release artifact contract.
- Python package API compatibility is still documented as legacy/reference.
- The test harness still imports `ixf_toolbox` modules for packaging, fixtures,
  and reference-contract coverage.
- CI and release workflows still validate Python packaging.
- Python deletion still requires explicit user approval after this report.

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

## Required Approval

Do not delete Python code in this release.

The next safe step is to decide whether the Python package API and Python
release artifacts remain supported. If they are intentionally deprecated, add a
separate staged release that updates tests, CI, release artifacts, and docs
before any deletion patch.
