# Go Full Python Replacement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Go `ixf` runtime fully replace the Python runtime while keeping Python available until every deletion gate is verified.

**Architecture:** Continue shipping Go as the default CLI/runtime and keep Python as legacy/reference until Go covers all user-facing behavior, all parity tests are Go-owned, and there is a tested removal path. Use fixture-only API tests with placeholder tenant URLs and fake IDs; do not use real OKR/document data in repo tests or docs.

**Tech Stack:** Go 1.24+, Python 3.11 legacy/reference, pytest, ruff, Go test, GitHub Actions release workflow.

## Global Constraints

- Keep Python code in the repository until a separate deletion-readiness report proves that Go owns every runtime behavior and rollback requirement.
- Do not include real tenant URLs, document IDs, OKR IDs, person names, cookies, CSRF tokens, passwords, or private response payloads.
- Use placeholder domains such as `https://tenant.example.test` and fake tokens such as `csrf-fixture`, `session-fixture`, and `okr-fixture-200`.
- Every behavior change uses TDD: write a failing test, observe red, implement minimal Go code, verify green.
- GitHub network operations use `HTTP_PROXY=socks5://127.0.0.1:7890`, `HTTPS_PROXY=socks5://127.0.0.1:7890`, and `ALL_PROXY=socks5://127.0.0.1:7890`.
- Report Python deletion readiness only after full local verification, GitHub CI success, and a fresh comparison against the Python reference surface.

---

### Task 1: OKR Target Objective Create-By-Index

**Files:**
- Modify: `internal/okr/okr.go`
- Modify: `tests/test_go_poc.py`
- Modify: `CHANGELOG.md`
- Modify: `README.md`
- Modify: `README.en.md`

**Interfaces:**
- Consumes: Go `ixf okr write --objective-index N --apply` selected Objective write flow.
- Produces: Go can create O(N) when `N == currentObjectiveCount + 1`, then create KRs and publish the new Objective.

- [x] **Step 1: Write the failing regression test**

Add `tests/test_go_poc.py::test_go_ixf_okr_write_apply_creates_next_objective_by_index`, using only `tenant.example.test`-style fixture data. The test must expect the API sequence `csrf`, `detail`, `version`, `create_objective`, `objective`, `create_kr`, `kr_text`, `create_kr`, `kr_text`, `publish`, `detail`.

- [x] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py::test_go_ixf_okr_write_apply_creates_next_objective_by_index -q
```

Expected: FAIL with `ERROR O3 was not found`.

- [x] **Step 3: Implement the minimal Go create path**

In `internal/okr/okr.go`, allow `ObjectiveIndex == len(objectives)+1`, call `POST /okrx/api/draft_v2/objective/`, read `objective_id` or `objectiveId`, then reuse the existing Objective text update, KR creation, publish, and final verification path.

- [x] **Step 4: Verify focused OKR paths**

Run:

```bash
python -m pytest tests/test_go_poc.py::test_go_ixf_okr_write_apply_creates_next_objective_by_index tests/test_go_poc.py::test_go_ixf_okr_write_apply_updates_target_objective_by_index -q
```

Expected: both tests pass.

### Task 2: OKR Draft Version Retry Parity

**Files:**
- Modify: `internal/okr/okr.go`
- Modify: `tests/test_go_poc.py`

**Interfaces:**
- Consumes: Python `DraftVersionCache`, `draft_version_from_payload`, and `NeedVersionRefresh` behavior from `src/ixf_toolbox/core/okr/writer.py`.
- Produces: Go OKR write reuses returned draft versions and retries stale-version API responses without exposing private payloads.

- [x] **Step 1: Write stale-version fixture test**

Add a Go CLI test where one draft mutation returns JSON `{"code":100001,"message":"stale version"}` on the first call and succeeds after `GET /okrx/api/okr/<okrId>/version/` returns a newer fixture version.

- [x] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py::test_go_ixf_okr_write_apply_retries_stale_draft_version -q
```

Expected: FAIL because Go currently treats every non-zero OKR API code as terminal.

- [x] **Step 3: Implement version cache and retry**

Add a small Go draft-version helper that stores versions returned in `data.draft_version`, `data.draftVersion`, `data.okr_draft_version`, `data.okrDraftVersion`, or `data.version`; clear and refetch once when code `100001` is returned.

- [x] **Step 4: Verify focused and full OKR write tests**

Run:

```bash
python -m pytest tests/test_go_poc.py -k "okr_write" -q
go test -count=1 ./internal/okr ./cmd/ixf
```

Expected: all selected tests pass.

### Task 3: OKR Full Specs And Prune Parity

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `internal/okr/okr.go`
- Modify: `tests/test_go_poc.py`
- Modify: `README.md`
- Modify: `README.en.md`

**Interfaces:**
- Consumes: Python `write_specs`, `delete_published_objective`, `order_krs`, and `--prune` behavior.
- Produces: Go can run OKR writes without `--objective-index`, update or create Objectives by text, append/reorder KRs when not pruning, delete missing Objectives/KRs only with explicit `--prune`, and publish each changed Objective.

- [x] **Step 1: Write non-index dry-run and apply tests**

Add fixture tests for multi-Objective input, existing Objective match by text, new Objective creation, KR append/order, and dry-run output that does not require cookies.

- [x] **Step 2: Write prune tests**

Add fixture tests where `--prune --apply` deletes non-input Objectives and replaces target KRs, while the same input without `--prune` preserves extra Objectives and KRs.

- [x] **Step 3: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py -k "okr_write and (prune or full_specs)" -q
```

Expected: FAIL because Go currently requires `--objective-index` for apply.

- [x] **Step 4: Implement Go full-spec writer**

Split `internal/okr/okr.go` into focused helpers only if the file becomes difficult to reason about: detail summary, objective create/update/delete, KR create/update/delete/order, publish, and verification.

- [x] **Step 5: Verify OKR parity**

Run:

```bash
python -m pytest tests/test_go_poc.py -k "okr" -q
go test -count=1 ./...
```

Expected: OKR read, targeted write, full-spec write, and prune tests pass.

### Task 4: Runtime Parity Matrix And Python Dependency Audit

**Files:**
- Create: `docs/go-python-parity.md`
- Modify: `tests/test_engineering_assets.py`
- Modify: `README.md`
- Modify: `README.en.md`

**Interfaces:**
- Consumes: current Go command surface and Python package API surface.
- Produces: explicit checklist of Go-owned behaviors, Python-only behaviors, intentional legacy/reference behaviors, and deletion blockers.

- [x] **Step 1: Write documentation contract test**

Add a test that requires `docs/go-python-parity.md` to contain sections `Go-owned Runtime`, `Python Legacy/Reference`, `Deletion Gates`, and `Known Blockers`.

- [x] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_engineering_assets.py -q
```

Expected: FAIL because the parity document does not exist yet.

- [x] **Step 3: Write the parity matrix**

Document every command family: `docs read`, `docs publish`, `okr read`, `okr write`, `cookies export`, `doctor`, `setup skills`, `update check`, `update self`, and `update skills`. Mark Python-only package API as legacy unless a current consumer requires it.

- [x] **Step 4: Verify docs contract**

Run:

```bash
python -m pytest tests/test_engineering_assets.py -q
python -m ruff check .
```

Expected: tests and lint pass.

### Task 5: Python Removal Readiness Report

**Files:**
- Create: `docs/python-removal-readiness.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/migration-from-legacy.md`

**Interfaces:**
- Consumes: all Go parity tests and `docs/go-python-parity.md`.
- Produces: a human-readable report that says whether Python can be deleted, and exactly which files would be removed in a later approved change.

- [x] **Step 1: Write deletion-gate checklist**

Require these gates to be true before recommending deletion: Go owns every CLI runtime path; all skills call Go `ixf`; no tests require Python core implementations except packaging/reference tests; release artifacts include Go binaries for supported platforms; rollback story does not require in-repo Python implementation; docs no longer tell users to install Python except for archived releases.

- [x] **Step 2: Run full verification**

Run:

```bash
go test -count=1 ./...
go vet ./...
python -m pytest -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Report to the user before deletion**

If every gate passes, report that Python code can be considered for deletion and wait for explicit approval. If any gate fails, report the exact blocker and keep Python.

### Task 6: Python Code Deletion Only After Approval

**Files:**
- Modify: `pyproject.toml`
- Delete or archive: `src/ixf_toolbox/`
- Modify: `tests/`
- Modify: `.github/workflows/`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/migration-from-legacy.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: explicit user approval after Task 5.
- Produces: a Go-only repo/runtime with no Python runtime implementation.

- [ ] **Step 1: Stop if approval is missing**

Do not delete Python code without a direct user instruction after the readiness report.

- [ ] **Step 2: Write failing removal contract tests**

Add tests that assert the documented install path, CI workflows, and release assets no longer depend on Python runtime packages.

- [ ] **Step 3: Remove Python runtime and update CI**

Remove Python runtime code only after tests describe the new Go-only contract. Keep Python test harness only if the repo still intentionally uses pytest for fixture orchestration.

- [ ] **Step 4: Verify and release**

Run full verification, publish a release with changelog notes, and confirm GitHub CI/release workflows pass.
