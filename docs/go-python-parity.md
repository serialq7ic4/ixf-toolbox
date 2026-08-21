# Go / Python Runtime Parity

This document records the current runtime ownership boundary for `ixf-toolbox`.
The repository is now a Go-only runtime repository: Go owns the installed CLI
runtime, release artifacts, and agent skill execution path.

## Go-owned Runtime

The GitHub Release Go binary is the default and supported runtime for new
installs and for the agent skills installed by `ixf setup skills`.

| Command family | Go ownership | Notes |
|---|---|---|
| `docs read` | Owned | Reads authorized remote documents into local artifacts; local Markdown support is for explicit artifact generation and chunking diagnostics, not the default agent path for ordinary local file reading. |
| `docs publish` | Owned | Creates a new docx from Markdown with dry-run-first and explicit `--apply` semantics. |
| `docs update` | Owned | Updates an existing docx body from Markdown with dry-run-first, `replace_body`, complex-block safeguards, and explicit `--apply` semantics. |
| `docs table append-row` | Owned | `ixf docs table append-row --dry-run` plans one native docx table row append; `--apply` writes table cells, uploads local PNG/JPEG/SVG image cells to `docx_image`, binds image blocks, and verifies by readback. |
| `sheets read` | Owned | `ixf sheets read` reads direct authorized sheets links as Markdown/TSV through the sheet client-vars API. |
| `sheets update` | Owned | `ixf sheets update --dry-run` plans TSV cell updates; `ixf sheets update --apply` writes confirmed cells through the sheet user_changes API and verifies by readback. |
| `bitable inspect/read` | Owned | `ixf bitable inspect/read` resolves direct bitable, wiki bitable, and docx embedded bitable URLs into safe metadata without printing full tokens. |
| `bitable record create` | Owned | `ixf bitable record create --dry-run` plans new records appended to the current view by default, including local file paths for attachment fields and planned view index; `ixf bitable record create --apply` creates confirmed records through API-only RCE writes, uploads attachment files, and verifies by clientvars readback. |
| `bitable attach` | Owned | `ixf bitable attach --dry-run` plans attachment uploads into existing bitable attachment fields; `ixf bitable attach --apply` uploads local files, appends them to the matched existing attachment field through API-only RCE writes, and verifies by clientvars readback. |
| `okr read` | Owned | Reads authorized OKR pages through the OKR detail APIs. |
| `okr write` | Owned | Writes confirmed Objective / KR JSON, including index-targeted, full-spec, and explicit prune flows. |
| `cookies export` | Owned | Exports local desktop-session cookies on macOS and CI-covered Windows providers. |
| `doctor` | Owned | Reports runtime, skill, and cookie metadata without printing cookie values. |
| `setup skills` | Owned | Installs Codex and Claude Code skill wrappers that call the local `ixf` binary. |
| `setup deps` | Owned | Dry-runs or explicitly installs optional Mermaid rendering dependencies; desktop/browser login dependencies remain diagnostic-only. |
| `update check` | Owned | Checks the latest GitHub Release without mutating local files. |
| `update self` | Owned | Plans or applies local binary/package replacement with explicit `--apply`. |
| `update skills` | Owned | Refreshes installed local skill wrappers. |

Markdown Mermaid image publishing remains in the Go CLI path. The Go runtime
detects Mermaid fences, creates docx image blocks, and invokes external Mermaid
CLI `mmdc` for SVG-first/PNG-fallback rendering during confirmed `--apply`
writes; dry-runs and applies probe `mmdc` with a minimal diagram so missing
Puppeteer browser dependencies such as `chrome-headless-shell` are reported
before remote writes. It does not use a Python renderer or fallback.

## Test Harness

The repository test harness is Go-only:

- `go test ./...` covers unit, integration, CLI contract, and repository
  contract tests.
- `go vet ./...` is the supported static analysis gate in CI and release
  workflows.
- Release note extraction uses shell tooling in GitHub Actions instead of a
  repository Python script.

There is no Python package API, wheel, sdist, or Python runtime implementation.
Direct Python package API callers must migrate to the Go CLI.

## No Legacy Fallback

All current docs, wiki, docx, sheets, OKR, cookie, setup, update, and Messenger
workflows use Go `ixf` only. Do not use Python fallback, Python-compatible
readers, Python-compatible writers, `ixfdoc`, or `ixfwrite`. Old changelog
entries and `docs/superpowers/` implementation plans are historical records, not
current routing guidance.

## Deletion Gates

Python runtime deletion is complete:

- Go owns every documented CLI command family and every installed skill calls Go.
- Fixture parity covers document read/publish, OKR read/write, cookie export,
  diagnostics, setup, and update flows.
- No user-facing docs recommend Python for new installs.
- CI and release workflows publish supported Go binaries and do not require the
  Python runtime implementation for CLI behavior.
- Python package API is removed.
- `docs/python-removal-readiness.md` records the final deletion state.

## Known Blockers

No known blockers remain for Python runtime deletion.

Future runtime work should be implemented in Go and covered by Go tests or CLI
contract tests.
