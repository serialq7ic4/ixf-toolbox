# Docs Update Release Train Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for code changes. Use superpowers:verification-before-completion before each commit, tag, or release claim.

**Goal:** Ship `ixf docs update` as a dry-run-first, API-only existing-docx body replacement flow.

**Architecture:** Keep `docs publish` create-only. Add `docs update` as a separate command that reuses the docx API session, Markdown parsing, block generation, and verification code while targeting an existing docx token instead of creating a new document.

**Tech Stack:** Go 1.24+, standard library HTTP/JSON, existing `internal/docspublish`, existing CLI command routing and repository contract tests.

## Global Constraints

- Go-only runtime. Do not add Python code or Python test harnesses.
- Do not call `ixfdoc` or `ixfwrite`.
- Do not use Playwright for docs update.
- Do not write real remote documents in automated tests; use `httptest`.
- Do not commit cookies, CSRF tokens, private URLs, document IDs, private response payloads, or generated private artifacts.
- Run `gofmt`, `go test ./...`, `go vet ./...`, `git diff --check`, and release smoke before each release tag.
- Each version in the release train gets its own commit, tag, and GitHub Release.

---

## Task 1: v3.8.1 Docs Writer Boundary Fix

**Files:**
- Modify: `repository_contract_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `skills/codex/ixf-docs-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-docs-writer/SKILL.md`
- Modify: `skills/codex/using-ixf-toolbox/SKILL.md`
- Modify: `skills/claude-code/using-ixf-toolbox/SKILL.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `VERSION`
- Modify: `cmd/ixf/main.go`
- Modify: `CHANGELOG.md`

**Deliverable:** `publish` is explicitly create-only in skill/docs/contracts and dry-run JSON reports `operation:create_docx`.

**Verification:**
- `go test ./...`
- `go vet ./...`
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.8.1" -o /tmp/ixf-go ./cmd/ixf`
- `scripts/smoke-go-binary.sh /tmp/ixf-go 3.8.1`
- `git diff --check`

## Task 2: v3.9.0 Docs Update Dry-Run

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Deliverable:** `ixf docs update <file.md> --url <docx-url> --dry-run` reads target state, reports `operation:update_docx`, `mode:replace_body`, `destructive:true`, existing and planned block counts, and refuses non-docx URLs.

**Verification:**
- `go test ./...`
- `go vet ./...`
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.9.0" -o /tmp/ixf-go ./cmd/ixf`
- `scripts/smoke-go-binary.sh /tmp/ixf-go 3.9.0`
- `git diff --check`

## Task 3: v3.10.0 Docs Update Apply

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Deliverable:** `ixf docs update <file.md> --url <docx-url> --apply` replaces supported existing body blocks through `user_change`, verifies required text, and rejects complex existing content by default.

**Verification:**
- `go test ./...`
- `go vet ./...`
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.10.0" -o /tmp/ixf-go ./cmd/ixf`
- `scripts/smoke-go-binary.sh /tmp/ixf-go 3.10.0`
- `git diff --check`

## Task 4: v3.11.0 Docs Update Stabilization

**Files:**
- Modify: `repository_contract_test.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `skills/codex/ixf-docs-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-docs-writer/SKILL.md`
- Modify: `docs/go-python-parity.md`
- Modify: `docs/release.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Deliverable:** docs update has hardened diagnostics, contract coverage, skill guidance, and a documented manual smoke process. If complex replacement override is added, it must be explicit and destructive.

**Verification:**
- `go test ./...`
- `go vet ./...`
- `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.11.0" -o /tmp/ixf-go ./cmd/ixf`
- `scripts/smoke-go-binary.sh /tmp/ixf-go 3.11.0`
- `git diff --check`
