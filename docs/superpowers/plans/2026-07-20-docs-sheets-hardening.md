# Docs and Sheets Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ixf Toolbox reliable for agent-discovered help, Markdown table preservation, and API-only sheet write workflows.

**Architecture:** Ship the work in small releases. First harden existing CLI and docx writer contracts without adding new remote mutation surfaces. Then add a dedicated sheets domain instead of overloading `docs update`, with dry-run-first writes and fixture-backed API contracts.

**Tech Stack:** Go 1.24+, standard library HTTP/JSON, existing `cmd/ixf`, `internal/docspublish`, `internal/docslocal`, `internal/docx`, and new focused sheets package if required.

## Global Constraints

- Go `ixf` only; do not call `ixfdoc`, `ixfwrite`, Python fallback readers, or Python-compatible writers.
- All remote writes must be explicit `--apply`; dry-run must not mutate.
- Do not commit cookies, CSRF tokens, private URLs, document IDs, internal response payloads, or generated private artifacts.
- Each release must include tests, changelog/version updates, commit, tag, push, and release workflow confirmation.
- Main branch development is approved by the user for this release train.

---

### Task 1: CLI Help Contract and Markdown Table Preservation

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

**Interfaces:**
- Consumes: existing `run(args, stdout, stderr) int`.
- Produces: subcommand help handling for `-h`, `--help`, and `help`; table Markdown converted to non-empty docx content.

- [ ] Write failing tests for `docs read --help`, `okr read --help`, and flag package subcommand help returning exit `0` on stdout.
- [ ] Write failing tests proving Markdown table dry-run counts table content and apply payload contains table cell text.
- [ ] Implement targeted help printers and help detection before flag parsing.
- [ ] Implement Markdown table parsing as stable content-preserving callout/list blocks or real table blocks; do not emit empty callouts.
- [ ] Run targeted tests and full `go test ./...`.
- [ ] Bump to `3.11.3`, update changelog, commit, tag, push, and confirm release.

### Task 2: Update Verification Hardening

**Files:**
- Modify: `internal/docspublish/publish.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `docs/docs-update.md`
- Modify: `skills/codex/ixf-docs-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-docs-writer/SKILL.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

**Interfaces:**
- Consumes: `UpdateMarkdown` and `verify`.
- Produces: dry-run/apply metadata that reports Markdown table handling and rejects or warns on empty callout verification failures.

- [ ] Write failing tests for dry-run table metadata and apply verification detecting empty callout regressions.
- [ ] Add table handling metadata to publish/update payloads.
- [ ] Harden `verify` so it reports `emptyCalloutCount`, table downgrade counts, and required text misses.
- [ ] Update docs and skills to tell agents not to trust `ok=true` alone when structural warnings exist.
- [ ] Run targeted tests and full `go test ./...`.
- [ ] Bump, commit, tag, push, and confirm release.

### Task 3: Sheets Read/Write API Surface

**Files:**
- Create: `internal/sheets/sheets.go`
- Create: `internal/sheets/sheets_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `docs/agent-routing.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

**Interfaces:**
- Consumes: authenticated cookie JSON and existing sheet client vars read shape.
- Produces: `ixf sheets read <url>` and `ixf sheets update --url <url> --range A1 --input <file.tsv> --dry-run|--apply`.

- [ ] Write failing CLI tests for `ixf sheets --help`, `ixf sheets read`, and `ixf sheets update --dry-run` with no network mutation.
- [ ] Implement URL parsing for direct sheets links and embedded sheet descriptors.
- [ ] Implement TSV read/write planning with explicit target range and row/column counts.
- [ ] Implement fixture-backed API apply sequence only after confirming the live request shape from captured or existing evidence.
- [ ] Update routing docs and README to route sheet writes to `ixf sheets update`, not `docs update`.
- [ ] Run targeted tests and full `go test ./...`.
- [ ] Bump, commit, tag, push, and confirm release.

### Task 4: Live Validation Against Session 2 Scenario

**Files:**
- Modify only docs/tests if validation reveals documentation or contract gaps.

**Interfaces:**
- Consumes: second session artifacts and target document state after user approval.
- Produces: a validated workflow that updates the previously unfinished embedded sheet content without Playwright/manual typing.

- [ ] Re-read the second session target document and identify embedded sheet tokens/ranges without exposing private content.
- [ ] Run `ixf sheets update --dry-run` against the intended cells using sanitized/local TSV.
- [ ] Apply only to approved target cells and read back with `ixf docs read --expand-sheets`.
- [ ] If live API differs from fixture assumptions, stop and use three-agent review before changing implementation.
- [ ] Run full regression tests, commit any validation-driven fixes, tag/push/release if code changed, then report ready for user acceptance.
