# Go Release Train Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish every staged `ixf-toolbox` Go migration release from the consolidated `v1.3.0` baseline through `v2.0.0`, with full verification at each release boundary.

**Architecture:** Keep Python as the v1.x reference runtime while migrating Go command surfaces behind the same `ixf` CLI contract. Each minor release must produce a tagged GitHub Release with wheel, source distribution, Go binaries, checksums, changelog notes, and passing CI before the next version starts.

**Tech Stack:** Python 3.11+, Go 1.24+, pytest, ruff, hatchling/build, GitHub Actions release workflow.

## Global Constraints

- Do not include real tenant URLs, document IDs, OKR IDs, person names, cookies, CSRF tokens, passwords, or private response payloads.
- Use placeholder domains such as `https://tenant.example.test` and fake tokens such as `csrf-fixture` or `session-fixture`.
- Every behavior change uses TDD: write failing tests, observe red, implement minimal code, verify green.
- Every stage release requires version bump, changelog section, install documentation update, full local verification, tag push, and GitHub release workflow confirmation.
- GitHub network operations use `HTTP_PROXY=socks5://127.0.0.1:7890`, `HTTPS_PROXY=socks5://127.0.0.1:7890`, and `ALL_PROXY=socks5://127.0.0.1:7890`.
- Work on `main` only for release commits and tags; use an isolated worktree for larger feature branches when release tagging is not the immediate task.

---

### Task 1: Consolidated v1.3.0 Release

**Files:**
- Modify: `pyproject.toml`
- Modify: `src/ixf_toolbox/__init__.py`
- Modify: `cmd/ixf/main.go`
- Modify: `CHANGELOG.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/migration-from-legacy.md`
- Test: `scripts/extract_changelog.py`

**Interfaces:**
- Consumes: current `Unreleased` Go migration notes in `CHANGELOG.md`.
- Produces: tag `v1.3.0`, GitHub Release `v1.3.0`, and install docs pointing at the `v1.3.0` wheel.

- [ ] **Step 1: Confirm current baseline scope**

Run:

```bash
git status --short --branch
git tag --sort=version:refname | tail -20
python -m pytest -q
go test -count=1 ./...
python -m ruff check .
python -m compileall -q src tests
```

Expected: clean `main`, tags ending at `v1.2.0`, and all checks exit 0.

- [ ] **Step 2: Update release metadata**

Set all runtime/package versions to `1.3.0`, move `CHANGELOG.md` `Unreleased` entries into `## 1.3.0 - 2026-07-15`, and leave a fresh empty `Unreleased` section.

- [ ] **Step 3: Update installation documentation**

Replace release wheel examples in `README.md`, `README.en.md`, and `docs/migration-from-legacy.md` from `v1.2.0` / `1.2.0` to `v1.3.0` / `1.3.0`.

- [ ] **Step 4: Verify release notes extraction**

Run:

```bash
python scripts/extract_changelog.py 1.3.0 CHANGELOG.md
```

Expected: non-empty notes for consolidated Go migration behavior.

- [ ] **Step 5: Run full release verification**

Run:

```bash
go test -count=1 ./...
go vet ./...
python -m pytest -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
# Run the configured sensitive-data scan from the release checklist.
rm -rf dist build
python -m build
scripts/smoke.sh
```

Expected: all commands exit 0 and `dist/` contains the Python wheel and source distribution.

- [ ] **Step 6: Commit, tag, push, and watch release**

Run:

```bash
git add pyproject.toml src/ixf_toolbox/__init__.py cmd/ixf/main.go CHANGELOG.md README.md README.en.md docs/migration-from-legacy.md docs/superpowers/plans/2026-07-15-go-release-train.md
git commit -m "chore: release v1.3.0"
HTTP_PROXY=socks5://127.0.0.1:7890 HTTPS_PROXY=socks5://127.0.0.1:7890 ALL_PROXY=socks5://127.0.0.1:7890 git push origin main
git tag v1.3.0
HTTP_PROXY=socks5://127.0.0.1:7890 HTTPS_PROXY=socks5://127.0.0.1:7890 ALL_PROXY=socks5://127.0.0.1:7890 git push origin v1.3.0
HTTP_PROXY=socks5://127.0.0.1:7890 HTTPS_PROXY=socks5://127.0.0.1:7890 ALL_PROXY=socks5://127.0.0.1:7890 gh run list --workflow Release --limit 3
```

Expected: release workflow for `v1.3.0` completes successfully.

### Task 2: v1.4.0 Docs Publish Apply Parity

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `tests/test_go_poc.py`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: existing Go `docs publish` dry-run planner and Python reference publisher behavior.
- Produces: Go `ixf docs publish --apply` API-only implementation with dry-run-first tests and safe fixture-only API coverage.

- [ ] **Step 1: Write failing CLI contract tests**

Add tests that prove `ixf docs publish --apply` performs mocked create/write/verify API calls, requires cookies only on apply, never mutates on dry-run, and redacts sensitive values in failures.

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py -q
```

Expected: tests fail because Go `docs publish --apply` is still unsupported.

- [ ] **Step 3: Implement minimal Go apply path**

Implement only the API calls and validation required by the failing tests, reusing existing Markdown parsing and cookie loading conventions.

- [ ] **Step 4: Verify green and release**

Run full release verification, bump to `1.4.0`, update changelog and install docs, commit `feat: add go docs publish apply`, tag `v1.4.0`, push with proxy, and confirm the release workflow passes.

### Task 3: v1.5.0 OKR Go API Parity

**Files:**
- Modify: `cmd/ixf/main.go`
- Create or modify: `internal/okr/`
- Modify: `tests/test_go_poc.py`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: Python reference OKR reader/writer API behavior and existing OKR fixture JSON.
- Produces: Go `ixf okr read` and `ixf okr write` API-only behavior with selected Objective protection and publish-after-edit semantics.

- [ ] **Step 1: Write failing OKR read/write CLI tests**

Add fixture-only tests for OKR URL parsing, read rendering, dry-run write planning, selected Objective update, KR create/update, prune opt-in, publish-after-edit, and secret-safe errors.

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py -q
```

Expected: tests fail because Go OKR command surface is not yet implemented.

- [ ] **Step 3: Implement minimal Go OKR API client**

Port only the tested Python reference behavior into focused Go files under `internal/okr/`, using fake fixture endpoints in tests.

- [ ] **Step 4: Verify green and release**

Run full release verification, bump to `1.5.0`, update changelog and install docs, commit `feat: add go okr api parity`, tag `v1.5.0`, push with proxy, and confirm the release workflow passes.

### Task 4: v1.6.0 Go Cookie Export and Diagnostics Parity

**Files:**
- Modify: `cmd/ixf/main.go`
- Create or modify: `internal/cookies/`
- Modify: `tests/test_go_poc.py`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: Python cookie export providers and doctor output contracts.
- Produces: Go `ixf cookies export` real local export behavior where platform support is available, plus diagnostics that remain secret-safe.

- [ ] **Step 1: Write failing cookie export and doctor parity tests**

Add platform-safe tests for successful fixture export, unavailable provider hints, output file metadata, and secret redaction.

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py -q
```

Expected: tests fail because Go cookie export still returns the safe unsupported path.

- [ ] **Step 3: Implement minimal Go cookie provider layer**

Port cookie loading/export seams without committing real browser or desktop session data.

- [ ] **Step 4: Verify green and release**

Run full release verification, bump to `1.6.0`, update changelog and install docs, commit `feat: add go cookie export parity`, tag `v1.6.0`, push with proxy, and confirm the release workflow passes.

### Task 5: v1.7.0 Contract Hardening Before v2

**Files:**
- Modify: `tests/test_go_poc.py`
- Modify: `tests/test_cli_contract.py`
- Modify: `tests/go_poc_support.py`
- Modify: `docs/release.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: all migrated Go command behavior.
- Produces: golden parity and regression coverage that can gate the v2 default runtime switch.

- [ ] **Step 1: Write failing contract hardening tests**

Add golden tests for command help, JSON output stability, Markdown/TSV output parity, update flow safety, skill install routing, and explicit write gating.

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_go_poc.py tests/test_cli_contract.py -q
```

Expected: tests expose any missing contract behavior before v2.

- [ ] **Step 3: Fix only contract gaps**

Patch Go or Python wrappers only where tests expose differences from the documented `ixf` surface.

- [ ] **Step 4: Verify green and release**

Run full release verification, bump to `1.7.0`, update changelog and install docs, commit `test: harden go migration contracts`, tag `v1.7.0`, push with proxy, and confirm the release workflow passes.

### Task 6: v2.0.0 Go Default Runtime Release

**Files:**
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/migration-from-legacy.md`
- Modify: `docs/supported-platforms.md`
- Modify: `docs/release.md`
- Modify: `CHANGELOG.md`
- Modify: `cmd/ixf/main.go`
- Modify: `src/ixf_toolbox/`
- Modify: `tests/`

**Interfaces:**
- Consumes: all v1.x parity releases and GitHub binary artifacts.
- Produces: `v2.0.0` where Go is documented as the default install/runtime path and Python remains legacy/reference.

- [ ] **Step 1: Write failing v2 install/runtime contract tests**

Add tests that prove docs and CLI advertise Go as default, Python as legacy/reference, skill setup remains stable, and write commands still require explicit `--apply`.

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_package_contract.py tests/test_go_poc.py -q
```

Expected: tests fail because v1.x still documents Python as the default path.

- [ ] **Step 3: Switch default runtime docs and package metadata**

Update install docs, migration docs, support docs, and CLI messaging to make Go binary installation the default while preserving Python fallback instructions.

- [ ] **Step 4: Run full regression and release**

Run full release verification, bump to `2.0.0`, update changelog and install docs, commit `feat: release go default runtime`, tag `v2.0.0`, push with proxy, confirm release workflow passes, then run a clean install smoke test from the GitHub Release artifact.

- [ ] **Step 5: Notify user for acceptance**

Report exact released versions, CI/release workflow URLs or run IDs, local regression commands, and remaining risks if any.
