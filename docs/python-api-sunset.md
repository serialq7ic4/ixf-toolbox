# Python API Sunset Policy

## Support Status

The Python package API is legacy/reference. The Go CLI is the supported runtime
for document, OKR, cookie, doctor, setup, and update workflows.

## No New Python Runtime Features

No new Python runtime features are allowed. New behavior must be implemented in
Go and covered by Go or CLI contract tests.

## Removal Direction

Python code may be removed in a future removal release only after release
artifacts, CI, tests, docs, and rollback policy no longer depend on the Python
runtime implementation.

## Removal Direction

The end state is a Go-only implementation. Delete the Python runtime in a
staged removal release after release artifacts, tests, CI, and documentation no
longer depend on it.
