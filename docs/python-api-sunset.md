# Python API Deletion Policy

## Support Status

The Python package API has been removed. The Go CLI is the supported runtime for
document, OKR, cookie, doctor, setup, and update workflows.

The repository no longer publishes Python wheels or source distributions and no
longer contains the Python runtime/package implementation.

## No New Python Runtime Features

No new Python runtime features are allowed. New behavior must be implemented in
Go and covered by Go tests or CLI contract tests.

## Final State

The repository no longer uses Python source files, pytest, ruff, or Python
maintenance scripts. It is not a user-facing runtime, package API, or test
harness.
