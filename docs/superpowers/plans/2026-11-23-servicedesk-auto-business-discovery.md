# ServiceDesk Auto Business Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ServiceDesk read queries and diagnostics work with only an API Key by discovering accessible business scopes through headerless userinfo.

**Architecture:** Add a userinfo-only request path that may omit `businessId`, while preserving the positive-ID invariant for every other API call. Let query and CLI layers represent automatic discovery as ID `0`, and keep the v1 JSON shape stable.

**Tech Stack:** Go standard library, `httptest`, repository CLI and pluginpack conventions, ServiceDesk HTTPS API.

**Spec:** `docs/superpowers/specs/2026-11-23-servicedesk-auto-business-discovery-design.md`

## Global Constraints

- Go-only and API-first; no Python or browser UI automation.
- Never log or serialize API Keys or private ticket content in tests and diagnostics.
- Ordinary list, get, create and withdraw requests still require a positive business ID.
- Preserve ServiceDesk JSON contract version 1; automatic discovery reports `bootstrapBusinessId: 0`.
- Release as bugfix version `3.32.1`.

## Review Focus

- A zero userinfo business ID must omit the header rather than send `businessId: 0`; Task 1 tests it.
- A negative userinfo business ID must remain invalid; Task 1 tests it.
- Automatically discovered list scopes must use each returned business ID; Task 2 tests it.
- Explicit/env/file business ID precedence must remain intact; Task 3 retains and extends those tests.
- A configured API Key with no business ID must cause doctor to probe, while missing credentials still skip; Task 3 tests both paths.

---

### Task 1: UserInfo Transport Boundary

**Files:**
- Modify: `internal/servicedesk/client.go`
- Test: `internal/servicedesk/client_test.go`

**Interfaces:**
- Produces: `(*Client).UserInfo(context.Context, int) (UserInfo, error)` where zero omits the header and positive values preserve current behavior.

- [ ] Add failing tests for zero omitting `businessId`, positive preserving it, and negative returning `invalid_request`.
- [ ] Run focused tests and confirm the new zero case fails because the current shared request helper requires a positive ID.
- [ ] Add a narrowly scoped request option/helper for headerless userinfo without weakening ordinary requests.
- [ ] Run `go test ./internal/servicedesk -run 'TestClientUserInfo' -count=1` and confirm it passes.

### Task 2: Query Automatic Discovery

**Files:**
- Modify: `internal/servicedesk/query.go`
- Test: `internal/servicedesk/query_test.go`

**Interfaces:**
- Consumes: zero-capable `Client.UserInfo` from Task 1.
- Produces: `QueryTickets` accepting `BootstrapBusinessID == 0`; negative IDs remain invalid.

- [ ] Add a failing query test that starts with zero, verifies headerless userinfo, and verifies list requests use discovered IDs.
- [ ] Change query validation to reject only nil clients and negative bootstrap IDs.
- [ ] Run focused package tests and confirm all query behavior passes.

### Task 3: CLI and Doctor Behavior

**Files:**
- Modify: `cmd/ixf/servicedesk_query.go`
- Modify: `cmd/ixf/servicedesk.go`
- Test: `cmd/ixf/servicedesk_query_test.go`
- Test: `cmd/ixf/servicedesk_test.go`

**Interfaces:**
- Produces: `list` and `search` proceed with bootstrap ID zero when no optional source is configured; `serviceDeskDiagnostics` probes userinfo with zero.

- [ ] Add failing list/search tests with no business ID source and assert the query runner receives zero.
- [ ] Add failing standalone/root doctor tests proving API probing occurs without a configured business ID.
- [ ] Preserve invalid configured-value errors and explicit/env/file precedence while treating absence as automatic discovery.
- [ ] Run `go test ./cmd/ixf -run 'TestServiceDesk' -count=1` and confirm it passes.

### Task 4: Guidance, Packaging, and Release Metadata

**Files:**
- Modify: `docs/superpowers/specs/2026-11-23-servicedesk-ticket-query-design.md`
- Modify: `README.md` and current ServiceDesk guidance as needed
- Modify: `skills/ixf-servicedesk-reader/SKILL.md`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`
- Regenerate: native Codex and Claude plugin packages

**Interfaces:**
- Produces: current guidance that does not instruct Agents to ask users for bootstrap IDs; release version `3.32.1`.

- [ ] Add or update contract tests so canonical/generated skills document automatic discovery and optional overrides.
- [ ] Correct current docs, bump the patch version, and add a dated changelog entry.
- [ ] Regenerate plugin packages using the repository's canonical generator and run its check mode.

### Task 5: Verification and Delivery

**Files:** none expected beyond verification artifacts excluded from git.

- [ ] Run focused tests, `go test ./... -count=1`, `go test -race ./...`, and `go vet ./...`.
- [ ] Run pluginpack check and Windows portability validation used by CI.
- [ ] Build a temporary binary and perform live read-only doctor/list acceptance with the business ID env/file deliberately absent; do not expose credentials or ticket content.
- [ ] Request an independent branch review and address findings.
- [ ] Commit, push through `127.0.0.1:7890`, open and merge a PR, tag `v3.32.1`, and verify the GitHub Release assets.
- [ ] Update the local `ixf` binary and native Codex/Claude plugin installations, then verify their versions.
