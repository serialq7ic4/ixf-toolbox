# Docs Structure Preflight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship v3.21.0 with safe docx/wiki structure preflight exposed through `ixf docs structure` and reused by docs read/write dry-runs.

**Architecture:** Reuse the existing `internal/docxgraph` graph builder and add a focused safe-summary layer. Expose that summary through a read-only publish/session wrapper for write dry-runs and through docslocal remote reads for manifests.

**Tech Stack:** Go 1.24+, standard library HTTP/JSON, existing `cmd/ixf`, `internal/docslocal`, `internal/docspublish`, and `internal/docxgraph` packages.

## Global Constraints

- Go `ixf` only; do not call `ixfdoc`, `ixfwrite`, Python fallback readers, or Python-compatible writers.
- Structure preflight is read-only and must not call write endpoints.
- Do not print cookies, CSRF tokens, raw private API payloads, full private URLs, full document tokens, or raw block IDs.
- Existing write APIs and apply payload behavior remain unchanged in v3.21.
- Dry-run outputs for existing-docx writes include safe structure metadata.
- Do not update the currently installed local Codex skill or PATH `ixf` until the v3.21 release is complete.

---

### Task 1: Safe Structure Summary Model

**Files:**
- Create: `internal/docxgraph/summary.go`
- Create: `internal/docxgraph/summary_test.go`

**Interfaces:**
- Consumes: `docxgraph.Graph`
- Produces: `func (g Graph) SafeSummary() map[string]any`

- [ ] **Step 1: Write failing summary tests**

Run: `go test ./internal/docxgraph`

Expected: FAIL because `SafeSummary` does not exist.

- [ ] **Step 2: Implement summary generation**

Add redacted block references, heading paths, section ranges, counts, complex
type diagnostics, sibling summaries, and duplicate-heading diagnostics.

- [ ] **Step 3: Verify graph tests**

Run: `go test ./internal/docxgraph`

Expected: PASS.

### Task 2: Structure Command And Remote Loader

**Files:**
- Modify: `internal/docspublish/patch.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`

**Interfaces:**
- Consumes: `func loadPatchState(rawURL, cookiesPath, spaceAPI string) (patchState, error)`
- Produces: `func Structure(rawURL, cookiesPath, spaceAPI string) (map[string]any, error)`

- [ ] **Step 1: Write failing CLI help and mocked command tests**

Run: `go test ./cmd/ixf -run 'TestLeafCommandHelp|TestCLIDocsStructure'`

Expected: FAIL because `docs structure` is not registered.

- [ ] **Step 2: Implement the read-only command**

Add `ixf docs structure <url> --json [--cookies PATH] [--space-api URL]` and
return the safe graph summary plus operation metadata.

- [ ] **Step 3: Verify command tests**

Run: `go test ./cmd/ixf -run 'TestLeafCommandHelp|TestCLIDocsStructure'`

Expected: PASS.

### Task 3: Read And Write Preflight Integration

**Files:**
- Modify: `internal/docslocal/local.go`
- Modify: `internal/docslocal/local_test.go`
- Modify: `internal/docslocal/remote_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `internal/docspublish/patch.go`
- Modify: `internal/docspublish/publish_test.go`
- Modify: `internal/docspublish/patch_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`

**Interfaces:**
- Consumes: `docxgraph.SafeSummary`
- Produces: manifest `structure` and `structureFile` fields for remote docs; `structure` fields in write dry-run payloads.

- [ ] **Step 1: Write failing read-manifest and dry-run tests**

Run: `go test ./internal/docslocal ./internal/docspublish ./cmd/ixf`

Expected: FAIL because structure metadata is missing from manifests and dry-run
payloads.

- [ ] **Step 2: Add structure metadata to read and write dry-runs**

Populate structure summaries for remote docx/wiki reads and existing-docx write
dry-runs. Write `<artifact>.structure.json` only when an output directory is
used.

- [ ] **Step 3: Verify package tests**

Run: `go test ./internal/docslocal ./internal/docspublish ./cmd/ixf`

Expected: PASS.

### Task 4: Docs, Version, And Release Verification

**Files:**
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/docs-update.md`
- Modify: `docs/agent-routing.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

**Interfaces:**
- Consumes: v3.21 CLI and dry-run behavior.
- Produces: repository docs that describe structure preflight without requiring users to name internal commands in natural prompts.

- [ ] **Step 1: Update docs and version metadata**

Set `VERSION` to `3.21.0` and add changelog notes.

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./...
go vet ./...
git diff --check
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.21.0" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go 3.21.0
```

Expected: all commands exit 0.

- [ ] **Step 3: Commit v3.21.0**

Commit implementation and documentation changes. Do not update installed local
skills or PATH `ixf` in this task.
