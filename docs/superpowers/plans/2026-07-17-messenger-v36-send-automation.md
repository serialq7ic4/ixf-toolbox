# Messenger v3.6 Send Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Go-native Messenger sending with pre-send target verification and mandatory fresh-session post-send verification.

**Architecture:** Keep CLI parsing in `cmd/ixf/main.go`; extend `internal/messenger` with send planning and a browser send request. `SendMessage` owns two profile clones, while `ChromedpAutomator.Send` owns DOM automation for open, type, send, and verify.

**Tech Stack:** Go 1.24, `chromedp`, existing local LarkShell profile/cookie infrastructure.

## Global Constraints

- No Python runtime or Python scripts may be reintroduced.
- Messenger must not open or mutate the live LarkShell profile directly.
- `send --apply` sends a real message and must require explicit target, mode, and message text.
- `send --dry-run` must not launch a browser.
- Target verification must succeed before typing.
- Fresh-session verification must succeed before reporting success.
- Result payloads must not echo full message bodies, cookie values, CSRF tokens, private IDs, screenshots, or raw DOM state.

---

### Task 1: Send Planning And Lifecycle

**Files:**
- Modify: `internal/messenger/messenger.go`
- Modify: `internal/messenger/messenger_test.go`

**Interfaces:**
- Produces: `type SendConfig struct`
- Produces: `type BrowserSendRequest struct`
- Produces: `type BrowserSendResult struct`
- Produces: `SendMessage(context.Context, SendConfig, Automator) (map[string]any, error)`

- [x] **Step 1: Write failing package tests** for validation, dry-run no automation, apply using two clones, no message echo in payload, sent/verified booleans, and clone cleanup.
- [x] **Step 2: Run package tests and confirm they fail** because send interfaces are missing.
- [x] **Step 3: Implement minimal planning and send lifecycle.**
- [x] **Step 4: Re-run package tests and confirm they pass.**

### Task 2: Chromedp Send Runner

**Files:**
- Modify: `internal/messenger/automation.go`
- Modify: `internal/messenger/selectors.go`
- Modify: `internal/messenger/match_test.go`

**Interfaces:**
- Consumes: `BrowserSendRequest`
- Produces: `ChromedpAutomator.Send`

- [x] **Step 1: Write failing unit tests** for message redaction helpers and send result matching helpers without launching a browser.
- [x] **Step 2: Implement editor typing, send trigger, local echo wait, and fresh-session verification actions.**
- [x] **Step 3: Re-run package tests and compile all packages.**

### Task 3: CLI, Skills, And Docs

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `repository_contract_test.go`
- Modify: `skills/codex/ixf-messenger-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-messenger-writer/SKILL.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/supported-platforms.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `ixf messenger send --to <target> --mode person|conversation --message <text> --dry-run --json`
- Consumes: `ixf messenger send --to <target> --mode person|conversation --message <text> --apply --json`

- [x] **Step 1: Write failing CLI and repository tests** for `messenger send`, diagnostics capabilities, docs, and writer skill copy.
- [x] **Step 2: Implement CLI flags and documentation updates.**
- [x] **Step 3: Re-run CLI and repository tests.**

### Task 4: Release

**Files:**
- Modify: `VERSION`
- Modify: `cmd/ixf/main.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: version `3.6.0`

- [x] **Step 1: Bump version metadata to `3.6.0`.**
- [x] **Step 2: Run `go test ./...`, `go vet ./...`, `git diff --check`, and release binary smoke.**
- [ ] **Step 3: Commit, tag, push, and verify the GitHub Release workflow.**
