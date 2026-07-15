# Python Removal Unblock Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove every blocker listed in `docs/python-removal-readiness.md`, then delete the Python implementation so the repository keeps only the Go implementation.

**Architecture:** Keep the Go binary as the default and only supported CLI runtime. Separate release metadata, release artifacts, and test harness concerns from the Python package before deleting any Python source. Python may remain as temporary test harness code only while its runtime coverage is ported to Go.

**Tech Stack:** Go 1.24+, Python 3.11/3.12 as temporary test harness, pytest, ruff, GitHub Actions, GitHub Releases.

## Global Constraints

- The end state is Go-only: delete `src/ixf_toolbox/` and all Python runtime implementation after the technical deletion gates are green.
- Do not delete `src/ixf_toolbox/` before a staged removal task updates the readiness report to ready.
- Keep every release stage independently committed, tagged, pushed, and published.
- Use TDD for every code or workflow behavior change: failing test first, observe red, implement, verify green.
- For pure documentation stages, run targeted docs tests, ruff, compileall, Go tests, and rely on the GitHub Release workflow for full remote artifact smoke.
- Do not commit real tenant URLs, document IDs, OKR IDs, person names, cookies, CSRF tokens, passwords, or private response payloads.
- GitHub network operations use `HTTP_PROXY=socks5://127.0.0.1:7890`, `HTTPS_PROXY=socks5://127.0.0.1:7890`, and `ALL_PROXY=socks5://127.0.0.1:7890`.
- Keep release notes specific to the current stage; do not batch multiple stages into one changelog section.

---

### Task 1: v2.5 Runtime-Neutral Version And Python Sunset Policy

**Files:**
- Create: `VERSION`
- Create: `docs/python-api-sunset.md`
- Modify: `pyproject.toml`
- Modify: `cmd/ixf/main.go`
- Modify: `src/ixf_toolbox/__init__.py`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/python-removal-readiness.md`
- Modify: `docs/release.md`
- Modify: `tests/test_engineering_assets.py`
- Modify: `tests/test_cli_contract.py`
- Modify: `tests/test_doctor.py`
- Modify: `tests/test_doctor_cli.py`
- Modify: `tests/test_go_poc.py`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: current version values from `pyproject.toml`, `cmd/ixf/main.go`, and `src/ixf_toolbox/__init__.py`.
- Produces: `VERSION` as the release version source that future Go-only releases can validate without depending on Python package metadata.

- [ ] **Step 1: Write failing version-source and sunset-policy tests**

Add these assertions to `tests/test_engineering_assets.py`:

```python
def test_runtime_neutral_version_file_matches_public_versions():
    version = read("VERSION").strip()

    assert version == "2.5.0"
    assert f'version = "{version}"' in read("pyproject.toml")
    assert f'__version__ = "{version}"' in read("src/ixf_toolbox/__init__.py")
    assert f'var version = "{version}"' in read("cmd/ixf/main.go")


def test_python_api_sunset_policy_documents_no_new_python_runtime_features():
    text = read("docs/python-api-sunset.md")

    for expected in [
        "## Support Status",
        "Python package API",
        "legacy/reference",
        "No new Python runtime features",
        "Go CLI is the supported runtime",
        "future removal release",
        "Go-only implementation",
    ]:
        assert expected in text
```

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_engineering_assets.py::test_runtime_neutral_version_file_matches_public_versions tests/test_engineering_assets.py::test_python_api_sunset_policy_documents_no_new_python_runtime_features -q
```

Expected: FAIL because `VERSION` and `docs/python-api-sunset.md` do not exist.

- [ ] **Step 3: Add `VERSION` and sunset policy**

Create `VERSION`:

```text
2.5.0
```

Create `docs/python-api-sunset.md`:

```markdown
# Python API Sunset Policy

## Support Status

The Python package API is legacy/reference. The Go CLI is the supported runtime
for document, OKR, cookie, doctor, setup, and update workflows.

## No New Python Runtime Features

Do not add new Python runtime features. New behavior must be implemented in Go
and covered by Go or CLI contract tests.

## Removal Direction

Python code may be removed in a future removal release only after release
artifacts, CI, tests, docs, and rollback policy no longer depend on the Python
runtime implementation.

## Removal Direction

The end state is a Go-only implementation. Delete the Python runtime in a
staged removal release after release artifacts, tests, CI, and documentation no
longer depend on it.
```

- [ ] **Step 4: Bump public versions and docs to v2.5.0**

Update:

```text
pyproject.toml -> version = "2.5.0"
src/ixf_toolbox/__init__.py -> __version__ = "2.5.0"
cmd/ixf/main.go -> var version = "2.5.0"
README.md / README.en.md / docs/release.md -> v2.5.0 install and release references
CHANGELOG.md -> ## 2.5.0 - 2026-07-15
```

Update `docs/python-removal-readiness.md` to reference `docs/python-api-sunset.md` as the API decision, while keeping `Status: Not ready for deletion`.

- [ ] **Step 5: Verify v2.5 locally**

Run:

```bash
python -m pytest tests/test_engineering_assets.py tests/test_cli_contract.py tests/test_doctor.py tests/test_doctor_cli.py -q
python -m pytest tests/test_go_poc.py::test_go_ixf_version_matches_python_release tests/test_go_poc.py::test_go_ixf_doctor_json_is_secret_safe_and_reports_go_runtime tests/test_go_poc.py::test_go_ixf_update_self_json_defaults_to_dry_run_with_fixture -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit and release v2.5.0**

Run:

```bash
git add VERSION docs/python-api-sunset.md pyproject.toml cmd/ixf/main.go src/ixf_toolbox/__init__.py README.md README.en.md docs/python-removal-readiness.md docs/release.md tests/test_engineering_assets.py tests/test_cli_contract.py tests/test_doctor.py tests/test_doctor_cli.py tests/test_go_poc.py CHANGELOG.md
git commit -m "docs: define python api sunset policy"
HTTP_PROXY=socks5://127.0.0.1:7890 HTTPS_PROXY=socks5://127.0.0.1:7890 ALL_PROXY=socks5://127.0.0.1:7890 git push origin main
git tag v2.5.0
HTTP_PROXY=socks5://127.0.0.1:7890 HTTPS_PROXY=socks5://127.0.0.1:7890 ALL_PROXY=socks5://127.0.0.1:7890 git push origin v2.5.0
```

Wait for CI and Release workflows. Confirm release assets before starting Task 2.

### Task 2: v2.6 Go-Only Release Artifacts

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `docs/release.md`
- Modify: `docs/python-removal-readiness.md`
- Modify: `tests/test_engineering_assets.py`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `pyproject.toml`
- Modify: `cmd/ixf/main.go`
- Modify: `src/ixf_toolbox/__init__.py`
- Modify: `tests/test_cli_contract.py`
- Modify: `tests/test_doctor.py`
- Modify: `tests/test_doctor_cli.py`
- Modify: `tests/test_go_poc.py`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `VERSION` created in Task 1.
- Produces: GitHub Release artifacts containing Go binaries and checksums only; Python package code remains in the repo for tests/reference but wheel/sdist are no longer release assets.

- [ ] **Step 1: Write failing release artifact contract test**

Update `tests/test_engineering_assets.py::test_release_workflow_validates_tag_builds_and_publishes_artifacts`:

```python
def test_release_workflow_validates_tag_builds_and_publishes_artifacts():
    text = read(".github/workflows/release.yml")

    assert "VERSION" in text
    assert "actions/setup-go" in text
    assert "go test ./..." in text
    assert "Build Go binary artifacts" in text
    assert "ixf_${RELEASE_VERSION}_${goos}_${goarch}" in text
    assert "scripts/smoke-go-binary.sh" in text
    assert "scripts/extract_changelog.py" in text
    assert "softprops/action-gh-release" in text
    assert "python -m build" not in text
    assert "ixf_toolbox-*.whl" not in text
```

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_engineering_assets.py::test_release_workflow_validates_tag_builds_and_publishes_artifacts -q
```

Expected: FAIL because the release workflow still builds Python distributions.

- [ ] **Step 3: Update release workflow**

Change `.github/workflows/release.yml`:

```yaml
- name: Validate tag and version
  env:
    RELEASE_TAG: ${{ github.ref_name }}
  run: |
    version="$(cat VERSION)"
    expected="v${version}"
    actual="${RELEASE_TAG}"
    if [ "${actual}" != "${expected}" ]; then
      echo "tag ${actual} does not match VERSION ${expected}" >&2
      exit 1
    fi
```

Remove:

```yaml
- name: Build distributions
  run: python -m build
```

Keep Python installed for `pytest` and `ruff` until the test harness is migrated.

- [ ] **Step 4: Update release docs and readiness report**

Update `docs/release.md`:

```markdown
`ixf-toolbox` uses tagged GitHub Releases with staged Go binary artifacts.
Python wheel and source distribution artifacts stopped being published in v2.6.0.
```

Update `docs/python-removal-readiness.md`:

```markdown
| Rollback no longer needs in-repo Python implementation | Partial | GitHub Releases no longer publish wheel/sdist artifacts, but Python source still exists for reference and tests. |
```

- [ ] **Step 5: Verify v2.6 locally**

Run:

```bash
python -m pytest tests/test_engineering_assets.py -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit and release v2.6.0**

Commit message:

```bash
git commit -m "ci: publish go-only release artifacts"
```

Tag and publish `v2.6.0`. After release, confirm the GitHub Release has `ixf_2.6.0_*` binaries and checksums, and does not have `.whl` or `.tar.gz` Python package artifacts.

### Task 3: v2.7 Python Runtime Import Audit Contract

**Files:**
- Create: `scripts/audit_python_runtime_imports.py`
- Create: `tests/python_runtime_imports_allowlist.txt`
- Modify: `docs/python-removal-readiness.md`
- Modify: `tests/test_engineering_assets.py`
- Modify: version/changelog files for `v2.7.0`

**Interfaces:**
- Consumes: current pytest import usage from `tests/`.
- Produces: a stable allowlist of tests that still import `ixf_toolbox`, so removal work can shrink it intentionally.

- [ ] **Step 1: Write failing audit test**

Add to `tests/test_engineering_assets.py`:

```python
def test_python_runtime_import_allowlist_is_current():
    result = subprocess.run(
        [sys.executable, "scripts/audit_python_runtime_imports.py"],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=True,
    )

    assert "python runtime import allowlist is current" in result.stdout
```

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_engineering_assets.py::test_python_runtime_import_allowlist_is_current -q
```

Expected: FAIL because the audit script does not exist.

- [ ] **Step 3: Add audit script**

Create `scripts/audit_python_runtime_imports.py`:

```python
from __future__ import annotations

from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
ALLOWLIST = ROOT / "tests" / "python_runtime_imports_allowlist.txt"
IMPORT_RE = re.compile(r"^(from|import)\s+ixf_toolbox(?:\.|\s|$)")


def current_importers() -> list[str]:
    paths: list[str] = []
    for path in sorted((ROOT / "tests").glob("test_*.py")):
        for line in path.read_text(encoding="utf-8").splitlines():
            if IMPORT_RE.match(line):
                paths.append(path.relative_to(ROOT).as_posix())
                break
    return paths


def main() -> int:
    expected = [
        line.strip()
        for line in ALLOWLIST.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.startswith("#")
    ]
    actual = current_importers()
    if actual != expected:
        print("Python runtime import allowlist is stale", file=sys.stderr)
        print("Expected:", expected, file=sys.stderr)
        print("Actual:", actual, file=sys.stderr)
        return 1
    print("python runtime import allowlist is current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **Step 4: Create the initial allowlist**

Create `tests/python_runtime_imports_allowlist.txt` from:

```bash
python - <<'PY'
from pathlib import Path
import re

root = Path(".")
pat = re.compile(r"^(from|import)\s+ixf_toolbox(?:\.|\s|$)")
for path in sorted((root / "tests").glob("test_*.py")):
    if any(pat.match(line) for line in path.read_text(encoding="utf-8").splitlines()):
        print(path.as_posix())
PY
```

- [ ] **Step 5: Verify v2.7 locally**

Run:

```bash
python -m pytest tests/test_engineering_assets.py -q
python scripts/audit_python_runtime_imports.py
python -m ruff check .
python -m compileall -q src tests
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit and release v2.7.0**

Commit message:

```bash
git commit -m "test: add python runtime import audit"
```

Tag and publish `v2.7.0`. Confirm CI and Release workflows pass.

### Task 4: v2.8 Port Non-Remote Python Utility Coverage To Go

**Files:**
- Modify: `internal/markdown/`
- Modify: `internal/cookies/`
- Modify: `internal/update/`
- Modify: `cmd/ixf/main.go`
- Modify: `tests/python_runtime_imports_allowlist.txt`
- Modify: `docs/python-removal-readiness.md`
- Modify: version/changelog files for `v2.8.0`

**Interfaces:**
- Consumes: allowlist from Task 3.
- Produces: fewer Python runtime import tests by moving local utility behavior to Go tests.

- [ ] **Step 1: Pick the first removable allowlist group**

Start with these test files:

```text
tests/test_core_docs_markdown_chunks.py
tests/test_core_docs_assets.py
tests/test_update.py
tests/test_update_cli.py
tests/test_setup.py
```

- [ ] **Step 2: Write equivalent Go tests**

Add Go tests covering the same user-visible behavior:

```text
internal/markdown/*_test.go for outline/chunk behavior
internal/docx/*_test.go or internal/markdown/*_test.go for image asset helpers
internal/update/*_test.go for update check/self planning
cmd/ixf/main_test.go for setup/update CLI contracts
```

- [ ] **Step 3: Observe red**

Run each new Go test before implementation:

```bash
go test -count=1 ./internal/markdown ./internal/update ./cmd/ixf
```

Expected: FAIL for missing or incomplete Go behavior.

- [ ] **Step 4: Implement minimal Go behavior**

Implement only what the new Go tests require. Do not expand feature scope.

- [ ] **Step 5: Remove replaced files from the Python import allowlist**

Edit `tests/python_runtime_imports_allowlist.txt` and remove only the files whose behavior now has Go coverage.

- [ ] **Step 6: Verify v2.8 locally**

Run:

```bash
python scripts/audit_python_runtime_imports.py
python -m pytest tests/test_engineering_assets.py tests/test_go_poc.py -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0 and the allowlist is smaller than in v2.7.

- [ ] **Step 7: Commit and release v2.8.0**

Commit message:

```bash
git commit -m "test: port local utility contracts to go"
```

Tag and publish `v2.8.0`.

### Task 5: v2.9 Port Remote API Reference Coverage To Go Fixtures

**Files:**
- Modify: `internal/docx/`
- Modify: `internal/docspublish/`
- Modify: `internal/okr/`
- Modify: `cmd/ixf/main.go`
- Modify: `tests/test_go_poc.py`
- Modify: `tests/python_runtime_imports_allowlist.txt`
- Modify: `docs/python-removal-readiness.md`
- Modify: version/changelog files for `v2.9.0`

**Interfaces:**
- Consumes: existing Python reference tests for docs reader/publisher, cookies, OKR reader/writer, and Go fixture helpers in `tests/test_go_poc.py`.
- Produces: Go-owned fixture coverage for all remote API behaviors needed before Python deletion.

- [ ] **Step 1: Identify remaining runtime import tests**

Run:

```bash
python scripts/audit_python_runtime_imports.py
cat tests/python_runtime_imports_allowlist.txt
```

Expected: the remaining list is mostly remote API reference coverage.

- [ ] **Step 2: Add missing Go fixture tests**

Add fixture tests to `tests/test_go_poc.py` for any remote behavior still only covered by Python imports. Use only `tenant.example.test`, fake IDs, and fake tokens.

- [ ] **Step 3: Observe red**

Run targeted tests:

```bash
python -m pytest tests/test_go_poc.py -k "docs or okr or cookies" -q
```

Expected: FAIL where Go behavior is missing.

- [ ] **Step 4: Implement missing Go behavior**

Implement only the missing behavior in:

```text
internal/docx/
internal/docspublish/
internal/okr/
cmd/ixf/main.go
```

- [ ] **Step 5: Shrink the allowlist again**

Remove each replaced Python test from `tests/python_runtime_imports_allowlist.txt`.

- [ ] **Step 6: Verify v2.9 locally**

Run:

```bash
python scripts/audit_python_runtime_imports.py
python -m pytest tests/test_go_poc.py tests/test_engineering_assets.py -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0 and the allowlist is limited to Python package/reference-only tests or empty.

- [ ] **Step 7: Commit and release v2.9.0**

Commit message:

```bash
git commit -m "test: port remote api contracts to go"
```

Tag and publish `v2.9.0`.

### Task 6: v2.10 Ready-Pending-Approval Report

**Files:**
- Modify: `docs/python-removal-readiness.md`
- Modify: `docs/go-python-parity.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/migration-from-legacy.md`
- Modify: `tests/test_engineering_assets.py`
- Modify: version/changelog files for `v2.10.0`

**Interfaces:**
- Consumes: Go-only release assets, Python sunset policy, and runtime import allowlist from earlier tasks.
- Produces: readiness report with `Status: Ready for Python implementation deletion` only if all technical blockers are cleared.

- [ ] **Step 1: Write failing readiness contract**

Update `tests/test_engineering_assets.py`:

```python
def test_python_removal_readiness_reports_ready_only_after_blockers_clear():
    text = read("docs/python-removal-readiness.md")

    assert "Status: Ready for Python implementation deletion" in text
    assert "Python wheel and sdist are still part of the release artifact contract" not in text
    assert "Python package API compatibility is still documented as legacy/reference" not in text
    assert "next release deletes the Python implementation" in text
```

- [ ] **Step 2: Observe red**

Run:

```bash
python -m pytest tests/test_engineering_assets.py::test_python_removal_readiness_reports_ready_only_after_blockers_clear -q
```

Expected: FAIL until the report is updated.

- [ ] **Step 3: Update readiness report**

Only set:

```markdown
Status: Ready for Python implementation deletion.
```

if these are true:

```text
GitHub Releases no longer publish Python wheel/sdist artifacts.
Docs no longer recommend or document Python package API support for new use.
tests/python_runtime_imports_allowlist.txt is empty or contains only explicitly retained test harness files.
CI and release workflows do not require Python runtime source for CLI behavior.
```

- [ ] **Step 4: Verify v2.10 locally**

Run:

```bash
python scripts/audit_python_runtime_imports.py
python -m pytest tests/test_engineering_assets.py tests/test_go_poc.py -q
python -m ruff check .
python -m compileall -q src tests
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0.

- [ ] **Step 5: Commit and release v2.10.0**

Commit message:

```bash
git commit -m "docs: mark python removal ready pending approval"
```

Tag and publish `v2.10.0`.

- [ ] **Step 6: Stop before the destructive deletion stage**

Do not delete Python in this task. Report the readiness status to the user before starting the destructive deletion release.

### Task 7: v3.0 Python Runtime Deletion

**Files:**
- Delete: `src/ixf_toolbox/`
- Modify: `pyproject.toml` or replace it with non-package tooling config if pytest/ruff remain.
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/release.md`
- Modify: `docs/migration-from-legacy.md`
- Modify: `docs/python-removal-readiness.md`
- Modify: `tests/`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: readiness status from Task 6.
- Produces: Go-only runtime repository with no in-repo Python runtime implementation.

- [ ] **Step 1: Confirm readiness is green before deletion**

Do not proceed unless `docs/python-removal-readiness.md` says `Status: Ready for Python implementation deletion`.

- [ ] **Step 2: Write failing Go-only repository contract tests**

Add/update tests asserting:

```python
def test_repository_no_longer_contains_python_runtime_package():
    assert not (ROOT / "src" / "ixf_toolbox").exists()


def test_release_workflow_is_go_only_after_python_deletion():
    text = read(".github/workflows/release.yml")
    assert "python -m build" not in text
    assert "pip install -e" not in text
    assert "go test ./..." in text
```

- [ ] **Step 3: Observe red**

Run:

```bash
python -m pytest tests/test_engineering_assets.py -q
```

Expected: FAIL because Python source still exists.

- [ ] **Step 4: Remove Python runtime and update workflows**

Remove Python runtime code and any tests that only validate the removed Python API. Keep pytest only if it remains useful as a fixture harness without importing `src/ixf_toolbox`.

- [ ] **Step 5: Verify v3.0 locally**

Run:

```bash
python -m pytest tests/test_engineering_assets.py tests/test_go_poc.py -q
python -m ruff check .
git diff --check
go test -count=1 ./...
go vet ./...
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit and release v3.0.0**

Commit message:

```bash
git commit -m "refactor: remove python runtime implementation"
```

Tag and publish `v3.0.0`. Confirm release assets are Go binaries and checksums only.
