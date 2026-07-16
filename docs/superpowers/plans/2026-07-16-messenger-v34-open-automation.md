# Messenger v3.4 Open Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Go-native Messenger browser automation for explicitly applied target opening and verification.

**Architecture:** Keep CLI parsing in `cmd/ixf/main.go`; move profile lifecycle and browser execution into `internal/messenger`. Tests exercise the automation boundary with a fake automator while production uses `chromedp` against a cloned profile.

**Tech Stack:** Go 1.24, `chromedp`, existing local LarkShell profile/cookie infrastructure.

## Global Constraints

- No Python runtime or Python scripts may be reintroduced.
- Messenger must not open or mutate the live LarkShell profile directly.
- Real message sending must not be exposed in `v3.4.0`.
- `open --apply` may mark a chat as read, so explicit `--apply` is required.
- Headless is the default; visible fallback requires `--allow-visible-fallback`.
- Diagnostics and errors must not leak cookie values, CSRF tokens, message text, private IDs, or raw profile content.

---

### Task 1: Automation Boundary

**Files:**
- Modify: `internal/messenger/messenger.go`
- Create: `internal/messenger/automation.go`
- Modify: `internal/messenger/messenger_test.go`

**Interfaces:**
- Produces: `type Automator interface { Open(context.Context, BrowserOpenRequest) (BrowserOpenResult, error) }`
- Produces: `OpenTarget(context.Context, OpenConfig, Automator) (map[string]any, error)`

- [x] **Step 1: Write failing tests** for `OpenTarget --apply` invoking a fake automator with a cloned profile, `--dry-run` not invoking it, missing mode/apply validation, clone cleanup, and keep-clone behavior.
- [x] **Step 2: Run the package tests and confirm they fail** because `OpenTarget` and `Automator` do not exist.
- [x] **Step 3: Implement the minimal automation boundary and payload shaping.**
- [x] **Step 4: Re-run package tests and confirm they pass.**

### Task 2: Chromedp Production Runner

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/messenger/automation.go`
- Create: `internal/messenger/selectors.go`
- Create: `internal/messenger/match_test.go`

**Interfaces:**
- Consumes: `BrowserOpenRequest`
- Produces: `ChromedpAutomator.Open`

- [x] **Step 1: Write failing unit tests** for title normalization/matching and cookie JSON redaction-safe parsing.
- [x] **Step 2: Add `chromedp` dependency.**
- [x] **Step 3: Implement headless default launch, optional visible fallback, Messenger wait, target open, and target verification.**
- [x] **Step 4: Run package tests and compile all packages.**

### Task 3: CLI And Skills

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `skills/codex/ixf-messenger-reader/SKILL.md`
- Modify: `skills/codex/ixf-messenger-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-messenger-reader/SKILL.md`
- Modify: `skills/claude-code/ixf-messenger-writer/SKILL.md`

**Interfaces:**
- Consumes: `ixf messenger open --to <target> --mode person|conversation --apply --json`

- [x] **Step 1: Write failing CLI and repository tests** for `--apply`, `--allow-visible-fallback`, and no-send wording.
- [x] **Step 2: Implement CLI flags and skill copy updates.**
- [x] **Step 3: Re-run CLI and repository tests.**

### Task 4: Release

**Files:**
- Modify: `VERSION`
- Modify: `cmd/ixf/main.go`
- Modify: `CHANGELOG.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/supported-platforms.md`

**Interfaces:**
- Produces: version `3.4.0`

- [x] **Step 1: Bump version metadata and changelog to `3.4.0`.**
- [x] **Step 2: Run `go test ./...`, `go vet ./...`, `git diff --check`, and release binary smoke.**
- [ ] **Step 3: Commit, tag, push, and verify the GitHub Release workflow.**
