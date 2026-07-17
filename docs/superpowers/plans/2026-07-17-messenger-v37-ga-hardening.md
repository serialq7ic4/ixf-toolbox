# Messenger v3.7 GA Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Ship `v3.7.0` as a Messenger GA-hardening release with safer diagnostics, clearer operator docs, and no new write surface.

**Architecture:** Keep Messenger automation in `internal/messenger` and CLI formatting in `cmd/ixf/main.go`. Add secret-safe remediation guidance to doctor payloads, then document Chrome-only headless operation, profile cloning, read-side effects, send verification, and troubleshooting.

**Tech Stack:** Go 1.24, `chromedp`, local LarkShell profile clone, GitHub Release Go binaries.

## Global Constraints

- Do not reintroduce Python runtime or Python scripts.
- Do not send real Messenger messages during automated tests.
- Do not print cookie values, CSRF tokens, private conversation IDs, raw DOM, screenshots, or message bodies in diagnostics.
- Keep automatic browser discovery Chrome/Chromium-only; do not add Edge fallback.
- Keep dry-run commands browser-launch-free.

---

### Task 1: Secret-Safe Doctor Remediation

**Files:**
- Modify: `internal/messenger/messenger.go`
- Modify: `internal/messenger/messenger_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`

**Interfaces:**
- Consumes: `messenger.Doctor(config Config) map[string]any`
- Produces: doctor payload field `remediation []string`; text doctor prints each item as `remediation <text>`

- [x] **Step 1: Write failing tests**

Add tests requiring `ixf messenger doctor` JSON/text to include actionable remediation when browser/profile/cookie prerequisites fail, while still redacting secret values.

- [x] **Step 2: Verify red**

Run:

```bash
go test ./internal/messenger ./cmd/ixf
```

Expected: FAIL because `remediation` is not present.

- [x] **Step 3: Implement minimal doctor remediation**

Add a small helper in `internal/messenger` that returns stable, generic guidance for unsupported platform, missing profile, missing Chrome/Chromium, and missing/invalid cookies. Print the list in text mode.

- [x] **Step 4: Verify green**

Run:

```bash
go test ./internal/messenger ./cmd/ixf
```

Expected: PASS.

### Task 2: Messenger GA Documentation

**Files:**
- Create: `docs/messenger.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/supported-platforms.md`
- Modify: `skills/codex/ixf-messenger-reader/SKILL.md`
- Modify: `skills/codex/ixf-messenger-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-messenger-reader/SKILL.md`
- Modify: `skills/claude-code/ixf-messenger-writer/SKILL.md`

**Interfaces:**
- Consumes: Messenger CLI commands already shipped through `v3.6.2`
- Produces: user-facing GA runbook covering prerequisites, dry-run/apply semantics, Chrome-only headless browser use, profile clone policy, read-side effects, send success criteria, and troubleshooting

- [x] **Step 1: Write doc contract test**

Add or update repository documentation tests to require `docs/messenger.md` and the README Messenger section to mention Chrome/Chromium-only discovery, cloned profile usage, read/open may mark chats as read, and send success booleans.

- [x] **Step 2: Verify red**

Run:

```bash
go test ./...
```

Expected: FAIL until docs are added.

- [x] **Step 3: Add documentation**

Write `docs/messenger.md`, link it from both READMEs, update supported-platforms, and align Codex/Claude Code Messenger skills with the GA rules.

- [x] **Step 4: Verify green**

Run:

```bash
go test ./...
```

Expected: PASS.

### Task 3: v3.7.0 Release

**Files:**
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: passing Go tests and docs from Tasks 1-2
- Produces: tag/release `v3.7.0` with Go binary artifacts and refreshed local skills

- [x] **Step 1: Bump version and changelog**

Set `VERSION` to `3.7.0` and document Messenger GA hardening under `CHANGELOG.md`.

- [x] **Step 2: Verify full release baseline**

Run:

```bash
go test ./...
go vet ./...
git diff --check
go run ./cmd/ixf --version
```

Expected: all commands exit 0 and version prints `ixf 3.7.0`.

- [x] **Step 3: Commit, tag, push, release**

Commit the changes, tag `v3.7.0`, push through the configured GitHub remote, wait for CI/release, and refresh local Codex skills with `ixf setup skills --runtimes codex --force --json`.
