# Python Removal Readiness

This report decides whether the repository can remove the Python runtime code in
the current release.

## Current Decision

Status: Not ready for deletion.

Go owns every documented CLI command family, and the Go binary is the default
runtime for new agent installs. Python still remains in this release only as a
temporary migration surface because tests and packaging contracts still import
Python modules.

Keep Python in this release.

## Deletion Gates

| Gate | Status | Evidence |
|---|---|---|
| Go owns every documented CLI command family | Pass | `docs/go-python-parity.md` marks each documented command family as Go-owned. |
| Installed skills call Go `ixf` | Pass | `ixf setup skills` installs wrappers around the local `ixf` command. |
| Release artifacts include supported Go binaries | Pass | The release workflow builds macOS, Windows, and Linux Go binaries plus checksums. |
| Tests no longer require Python runtime implementations | Blocked | Tests still import Python modules for package, fixture, and reference-contract coverage. |
| Rollback no longer needs in-repo Python implementation | Partial | GitHub Releases no longer publish Python wheel or sdist artifacts, but Python source still exists for reference and tests. |
| New-install docs avoid Python as the default | Pass | README files direct new users to the Go binary first. |
| Remaining Python package API users have a migration decision | Blocked | `docs/python-api-sunset.md` documents the Python package API as temporary migration surface pending deletion. |
| Destructive removal stage is reached | Blocked | Technical deletion gates are still blocked in this release. |

## Current Blockers

- The test harness still imports `ixf_toolbox` modules for packaging, fixtures,
  and reference-contract coverage; `tests/python_runtime_imports_allowlist.txt`
  is down to 2 files after moving local CLI/update/setup/doctor/cookie/local-docs/Markdown,
  document image asset, document converter, and remote document reader contracts
  to Go tests.
- Python package API deletion is not complete; direct import users must move to
  the Go CLI before the removal release.
- CI still validates the temporary Python source tree for reference coverage.
- Python deletion must wait for the staged removal release after technical gates
  are cleared.

## Files Covered By A Future Removal

A later approved removal would need to review these areas:

- `src/ixf_toolbox/` Python package modules.
- Python package metadata and build settings in `pyproject.toml`.
- Python-specific tests under `tests/`.
- CI steps that still import or validate Python source for reference coverage.
- README, migration, platform, privacy, and release documentation that mention
  the temporary Python migration surface.
- Any skill wrapper text that still references Python fallback behavior.

## Removal Direction

Do not delete Python code in this release.

The next safe step is to shrink `tests/python_runtime_imports_allowlist.txt` by
porting remaining OKR reference coverage and residual `tests/test_go_poc.py`
Python reference imports to Go fixtures, while keeping the Python source tree
until test and API blockers are cleared.
