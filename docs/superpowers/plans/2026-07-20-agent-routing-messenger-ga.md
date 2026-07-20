# Agent Routing And Messenger GA Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `v3.8.0` with explicit agent-routing diagnostics and Messenger GA stability metadata.

**Architecture:** Add small machine-readable contract objects to existing doctor payloads, then update docs and skill wrappers to point agents at the current Go-only routing boundary. Keep browser automation behavior unchanged.

**Tech Stack:** Go 1.24+, standard Go tests, existing `ixf` CLI, Markdown docs, Codex and Claude Code skill wrappers.

## Global Constraints

- No Python runtime, Python scripts, Python tests, `ixfdoc`, or `ixfwrite`.
- Do not change Messenger send semantics; success still requires `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and `verifiedPresent:true`.
- Do not print cookies, CSRF tokens, private URLs, private IDs, raw DOM, screenshots, or profile contents.
- Use TDD for behavior changes and keep commits small.

---

### Task 1: Agent Routing Diagnostic Contract

**Files:**
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `repository_contract_test.go`
- Create: `docs/agent-routing.md`
- Modify: `skills/codex/using-ixf-toolbox/SKILL.md`
- Modify: `skills/claude-code/using-ixf-toolbox/SKILL.md`

**Interfaces:**
- Consumes: existing `collectDiagnostics(cookiesPath string) map[string]any`.
- Produces: `agentRoutingStatus() map[string]any`, returned under `agentRouting`.

- [x] **Step 1: Write failing tests**

Add assertions that `ixf doctor --json` includes `agentRouting.goOnly=true`,
`agentRouting.backgroundRouting=true`, `agentRouting.defaultAmbiguousIntent="read-only"`,
and `agentRouting.currentGuidance` pointing at `AGENTS.md`, `docs/agent-routing.md`,
and `skills/*/*/SKILL.md`.

- [x] **Step 2: Verify failure**

Run: `go test ./cmd/ixf -run 'TestCollectDiagnostics|TestDoctor'`

Expected: FAIL because `agentRouting` is absent.

- [x] **Step 3: Implement minimal contract**

Add `agentRoutingStatus()` in `cmd/ixf/main.go` and include it in `collectDiagnostics`.
Extend text diagnostics with one line: `agent_routing go_only=true background=true default=read-only`.

- [x] **Step 4: Update routing docs and skills**

Create `docs/agent-routing.md`. Update both `using-ixf-toolbox` skill wrappers
to say users can describe tasks naturally and routing happens in the background.

- [x] **Step 5: Verify task**

Run: `go test ./cmd/ixf` and `go test .`

Expected: PASS.

### Task 2: Messenger GA Stability Diagnostics

**Files:**
- Modify: `cmd/ixf/main_test.go`
- Modify: `internal/messenger/messenger.go`
- Modify: `cmd/ixf/main.go`
- Modify: `docs/messenger.md`
- Modify: `docs/supported-platforms.md`

**Interfaces:**
- Consumes: existing `messenger.Doctor(config Config) map[string]any`.
- Produces: `messenger.stability` map in doctor JSON.

- [x] **Step 1: Write failing tests**

Add assertions that Messenger doctor JSON exposes `messenger.stability.operatingModel`,
`messenger.stability.macOS`, `messenger.stability.windows`, and four send success criteria.
Add a text-mode test that duplicate remediation lines are not emitted.

- [x] **Step 2: Verify failure**

Run: `go test ./cmd/ixf -run MessengerDoctor` and `go test ./internal/messenger`.

Expected: FAIL because stability metadata is absent and duplicate browser remediation still exists.

- [x] **Step 3: Implement minimal diagnostics**

Add `StabilityStatus(goos string) map[string]any` in `internal/messenger/messenger.go`.
Deduplicate remediation construction by removing the repeated browser line.
Print a compact `stability` line in text diagnostics.

- [x] **Step 4: Verify task**

Run: `go test ./cmd/ixf -run MessengerDoctor` and `go test ./internal/messenger`.

Expected: PASS.

### Task 3: v3.8.0 Release

**Files:**
- Modify: `VERSION`
- Modify: `cmd/ixf/main.go`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Produces: tagged release `v3.8.0`.

- [x] **Step 1: Bump version and docs**

Set `VERSION` and `cmd/ixf/main.go` to `3.8.0`. Update install examples and
runtime status text in both README files.

- [x] **Step 2: Full local verification**

Run: `gofmt -w cmd/ixf/main.go cmd/ixf/main_test.go internal/messenger/messenger.go repository_contract_test.go`
Run: `go test ./...`
Run: `go vet ./...`
Run: `git diff --check`
Run: `go run ./cmd/ixf --version`

Expected: all commands exit 0 and version prints `ixf 3.8.0`.

- [ ] **Step 3: Commit and release**

Commit, tag `v3.8.0`, push with `HTTP_PROXY` and `HTTPS_PROXY` set to
`http://127.0.0.1:7890`, then watch CI and Release workflows.
