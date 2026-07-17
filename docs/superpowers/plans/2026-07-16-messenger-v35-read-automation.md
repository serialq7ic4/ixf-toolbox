# Messenger v3.5 Read Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Go-native, read-only Messenger conversation extraction behind explicit `--apply`.

**Architecture:** Keep CLI parsing in `cmd/ixf/main.go`; extend the `internal/messenger` automation boundary from target opening to conversation reading. Tests fake the automator for lifecycle behavior while production uses `chromedp` DOM extraction.

**Tech Stack:** Go 1.24, `chromedp`, existing local LarkShell profile/cookie infrastructure.

## Global Constraints

- No Python runtime or Python scripts may be reintroduced.
- Messenger must not open or mutate the live LarkShell profile directly.
- Real message sending must not be exposed in `v3.5.0`.
- `read --apply` may mark chats as read, so explicit `--apply` is required.
- Headless is the default; visible fallback requires `--allow-visible-fallback`.
- Diagnostics and errors must not leak cookie values, CSRF tokens, private IDs, screenshots, or raw profile content.
- Message bodies may appear only in explicit `messenger read --apply` results.

---

### Task 1: Read Automation Boundary

**Files:**
- Modify: `internal/messenger/messenger.go`
- Modify: `internal/messenger/automation.go`
- Modify: `internal/messenger/messenger_test.go`

**Interfaces:**
- Produces: `type BrowserReadRequest struct`
- Produces: `type BrowserReadResult struct`
- Produces: `ReadMessages(context.Context, ReadConfig, Automator) (map[string]any, error)`

- [x] **Step 1: Write failing package tests** for dry-run not invoking automation, apply cloning profile, read options shaping, message output, no-send fields, secret safety, and clone cleanup.
- [x] **Step 2: Run package tests and confirm they fail** because read interfaces are missing.
- [x] **Step 3: Implement minimal planning and apply lifecycle.**
- [x] **Step 4: Re-run package tests and confirm they pass.**

### Task 2: Chromedp Read Extraction

**Files:**
- Modify: `internal/messenger/automation.go`
- Modify: `internal/messenger/selectors.go`
- Modify: `internal/messenger/match_test.go`

**Interfaces:**
- Consumes: `BrowserReadRequest`
- Produces: `ChromedpAutomator.Read`

- [x] **Step 1: Write failing unit tests** for recent card/message normalization helpers without launching a browser.
- [x] **Step 2: Implement DOM extraction actions for recent cards, unread filtering, chat opening, and message snippets.**
- [x] **Step 3: Re-run package tests and compile all packages.**

### Task 3: CLI, Skills, And Docs

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `repository_contract_test.go`
- Modify: `skills/codex/ixf-messenger-reader/SKILL.md`
- Modify: `skills/claude-code/ixf-messenger-reader/SKILL.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/supported-platforms.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `ixf messenger read --scope unread|recent --dry-run --json`
- Consumes: `ixf messenger read --scope unread|recent --apply --json`

- [x] **Step 1: Write failing CLI and repository tests** for `messenger read`, capabilities, docs, and skill copy.
- [x] **Step 2: Implement CLI flags and documentation updates.**
- [x] **Step 3: Re-run CLI and repository tests.**

### Task 4: Release

**Files:**
- Modify: `VERSION`
- Modify: `cmd/ixf/main.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: version `3.5.0`

- [x] **Step 1: Bump version metadata to `3.5.0`.**
- [x] **Step 2: Run `go test ./...`, `go vet ./...`, `git diff --check`, and release binary smoke.**
- [ ] **Step 3: Commit, tag, push, and verify the GitHub Release workflow.**
