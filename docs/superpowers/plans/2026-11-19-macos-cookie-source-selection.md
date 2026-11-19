# macOS Cookie Source Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make macOS cookie export select a usable authenticated desktop cookie source and give agents precise, secret-safe failure reasons.

**Architecture:** Keep `ixf cookies export` local and preserve the downstream `_csrf_token` contract. Refactor the macOS provider to enumerate only supported live-profile DB locations, evaluate candidates independently, and return typed diagnostics that the CLI and skills can act on without guessing process state.

**Tech Stack:** Go, modernc SQLite, existing `ixf` CLI and plugin package generator.

**Spec:** `docs/superpowers/specs/2026-11-19-macos-cookie-source-selection-design.md`

## Global Constraints

- Go runtime only; no Python fallback or UI automation.
- Do not alias `swp_csrf_token` to `_csrf_token`.
- Automatic discovery must exclude `profile_explorer-playwright-*` clones.
- Explicit `--cookies-db` must remain exact and must not fall back.
- Diagnostics must not expose cookie values or private API payloads.
- `ixf doctor --json` remains read-only.
- Treat this as a bug fix and bump only the patch version from `3.31.2` to `3.31.3`.

## Review Focus

- Multiple desktop users: deterministic priority and modification-time ordering must not make a clone eligible.
- One candidate has an unreadable schema or bad encryption: automatic mode must continue without hiding the rejection reason.
- An explicit bad DB: the command must fail on that DB rather than silently using another login.
- A CSRF cookie has Chromium expiry in the past: export must warn but keep the service-accepted session usable.
- JSON and text errors: neither may claim the desktop process is stopped or leak token/cookie values.

---

### Task 1: macOS Candidate Selection

**Files:**
- Modify: `internal/cookies/cookies.go`
- Modify: `internal/cookies/cookies_test.go`

**Interfaces:**
- Produces: `type ExportError struct`, `type CandidateDiagnostic struct`, and secret-safe accessors used by `cmd/ixf`.
- Produces: automatic macOS candidate enumeration and selection behind the existing `Export(ExportConfig) (map[string]any, error)` interface.

- [ ] **Step 1: Write failing tests for supported candidates and clone exclusion**

Add tests that construct root, Default, openplatform partition, and Playwright clone DB paths. Assert discovery includes the first three in stable source priority, excludes the clone, and sorts same-kind candidates by mtime.

- [ ] **Step 2: Run candidate tests and verify they fail because discovery still returns one root DB**

Run: `go test ./internal/cookies -run 'TestResolveMacOSCookieCandidates' -count=1`

- [ ] **Step 3: Implement candidate metadata and enumeration**

Introduce an internal candidate type with path and `sourceKind`; enumerate exact supported glob patterns only. Preserve an explicit `CookiesDB` as one `explicit` candidate.

- [ ] **Step 4: Write failing tests for fallback, explicit failure, stale CSRF, and secret-safe aggregate failure**

Assert automatic export skips missing-CSRF and decryption-failing candidates, selects a later partition, reports `candidateCount`, `sourceKind`, and `csrfExpired`, and writes no cookie values into success/error metadata. Assert explicit mode returns its own failure without fallback.

- [ ] **Step 5: Run export tests and verify the new selection assertions fail**

Run: `go test ./internal/cookies -run 'TestExportMacOS' -count=1`

- [ ] **Step 6: Implement independent candidate evaluation and typed failure**

Use the existing row reader and decryption code per candidate. Select the first candidate that fully exports and contains non-empty `_csrf_token`; collect only safe candidate summaries for rejected candidates. Add stale expiry metadata without rejecting the candidate.

- [ ] **Step 7: Run cookie tests**

Run: `go test ./internal/cookies -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/cookies/cookies.go internal/cookies/cookies_test.go
git commit -m "fix: select usable macOS cookie source"
```

### Task 2: CLI and Agent Diagnostic Contract

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `skills/using-ixf-toolbox/SKILL.md`
- Modify: `skills/ixf-docs-reader/SKILL.md`
- Modify: `skills/ixf-docs-writer/SKILL.md`
- Modify: `skills/ixf-okr-reader/SKILL.md`
- Modify: `skills/ixf-okr-writer/SKILL.md`

**Interfaces:**
- Consumes: `cookies.ExportError` and its secret-safe fields from Task 1.
- Produces: stable `cookie_source_unusable` JSON errors with `reason`, `hint`, `retryable`, and candidate diagnostics.

- [ ] **Step 1: Write failing CLI tests for structured and non-misleading failures**

Inject or construct a typed cookie export error and assert JSON includes the stable subtype and diagnostics, excludes cookie values, and says source failure does not establish desktop process state. Assert text output has an actionable remediation without “confirm the desktop client is logged in”.

- [ ] **Step 2: Run CLI tests and verify the generic error mapping fails assertions**

Run: `go test ./cmd/ixf -run 'TestCookiesExport' -count=1`

- [ ] **Step 3: Map typed cookie errors in `runCookies`**

Use `errors.As`; keep a generic fallback for unsupported provider and unexpected I/O errors. Do not change exit code `6`.

- [ ] **Step 4: Run CLI tests**

Run: `go test ./cmd/ixf -run 'TestCookiesExport' -count=1`
Expected: PASS.

- [ ] **Step 5: Update canonical skill authentication guidance**

Require agents to consume structured export diagnostics, forbid inferring process/login state from a missing token or `pgrep`, and direct users to refresh the target document only when the CLI remediation requests it.

- [ ] **Step 6: Commit**

```bash
git add cmd/ixf/main.go cmd/ixf/main_test.go skills
git commit -m "fix: make cookie failures actionable for agents"
```

### Task 3: Generated Plugins, Version, and Regression

**Files:**
- Modify: generated files under `plugins/codex/ixf-toolbox/` and `plugins/claude/ixf-toolbox/`
- Modify: `docs/supported-platforms.md`
- Modify: `docs/agent-routing.md`
- Modify: `VERSION` and generated version metadata

**Interfaces:**
- Consumes: canonical skill changes from Task 2.
- Produces: release-ready `3.31.3` plugin/runtime artifacts and documented authentication boundary.

- [ ] **Step 1: Document candidate selection and the process-state boundary**

State that macOS export checks supported live-profile DBs and that cookie export failure cannot prove whether the branded desktop executable is running.

- [ ] **Step 2: Bump patch version and regenerate plugin packages**

Run the repository's existing version workflow if present; otherwise update `VERSION` and the version-bearing generated inputs consistently. Then run `go run ./cmd/pluginpack`.

- [ ] **Step 3: Check generated packages and focused suites**

Run: `go run ./cmd/pluginpack --check`

Run: `go test ./internal/cookies ./cmd/ixf ./internal/pluginpack -count=1`

- [ ] **Step 4: Run the full Go suite and static checks**

Run: `go test ./... -count=1`

Run: `go vet ./...`

Run: `git diff --check`

- [ ] **Step 5: Perform a secret-safe live regression**

Use `ixf cookies export --provider auto` to a temporary `0600` file, then read the already-authorized wiki target from the incident. Assert both exit successfully, inspect only non-secret metadata, and truncate the temporary cookie file immediately afterward.

- [ ] **Step 6: Commit**

```bash
git add VERSION docs skills plugins
git commit -m "chore: prepare v3.31.3"
```

- [ ] **Step 7: Request independent review before push or release**

Review the full range from the design commit through `HEAD`, address findings with tests, and rerun Task 3 verification. GitHub operations must use proxy `127.0.0.1:7890`.

