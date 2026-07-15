# Python API Deletion Policy

## Support Status

The Python package API is a temporary migration surface. The Go CLI is the supported runtime
for document, OKR, cookie, doctor, setup, and update workflows.

This is not a long-term compatibility promise. The repository target is a
Go-only implementation.

## No New Python Runtime Features

No new Python runtime features are allowed. New behavior must be implemented in
Go and covered by Go or CLI contract tests.

## Removal Direction

The end state is a Go-only implementation. Delete the Python runtime in a
future removal release after release artifacts, tests, CI, and documentation no
longer depend on it.
