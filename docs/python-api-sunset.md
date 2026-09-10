# Python API Deletion Policy

## Support Status

The Python package API has been removed. The Go CLI is the supported runtime for
document, sheets, bitable, OKR, cookie, doctor, dependency, update, and Messenger
workflows. Native Codex and Claude plugins are the supported skill installation
path and use canonical sources from `skills/*/SKILL.md`.

The repository no longer publishes Python wheels or source distributions and no
longer contains the Python runtime/package implementation.

## No New Python Runtime Features

No new Python runtime features are allowed. New behavior must be implemented in
Go and covered by Go tests or CLI contract tests.

## Final State

The repository no longer uses Python source files, pytest, ruff, or Python
maintenance scripts. It is not a user-facing runtime, package API, or test
harness.

On first use, a native plugin resolves the Go `ixf` executable or presents a
bootstrap dry-run. It may install the user-local runtime only after explicit
confirmation and checksum verification, and it does not modify `PATH`.
`ixf doctor --json` is read-only. `ixf deps install --apply` is the only optional
dependency mutation path.
