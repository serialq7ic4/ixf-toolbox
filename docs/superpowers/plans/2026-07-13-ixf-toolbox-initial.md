# ixf-toolbox Initial Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first `ixf-toolbox` release as a stable `ixf` CLI and skill installer that delegates to existing reader/writer engines.

**Architecture:** `ixf_toolbox.cli` parses the unified command surface, `ixf_toolbox.delegate` forwards compatible subcommands to `ixfdoc` and `ixfwrite`, and `ixf_toolbox.setup` installs packaged skill wrappers. This release is a compatibility bridge; core implementation migration happens in later releases.

**Tech Stack:** Python 3.11+, argparse, subprocess, importlib.resources, hatchling, pytest, ruff.

## Global Constraints

- Distribution name is `ixf-toolbox`.
- CLI command is `ixf`.
- Do not include real tenant URLs, OKR IDs, document IDs, cookies, CSRF tokens, passwords, or private payloads.
- Do not remove or modify existing `ixunfei-docx-reader` or `ixunfei-docx-writer` repositories.
- Reader skills are read-only; writer skills require confirmed content and dry-run-first guidance.
- Release starts at `0.1.0`.

---

### Task 1: Project Skeleton And Contracts

**Files:**
- Create: `pyproject.toml`
- Create: `README.md`
- Create: `CHANGELOG.md`
- Create: `src/ixf_toolbox/__init__.py`
- Test: `tests/test_cli_contract.py`

**Interfaces:**
- Produces: `ixf_toolbox.__version__: str`
- Produces: `ixf_toolbox.cli.main(argv: list[str] | None = None) -> int`

- [ ] **Step 1: Write failing tests**

```python
from ixf_toolbox import __version__
from ixf_toolbox.cli import main


def test_version_constant_is_initial_release():
    assert __version__ == "0.1.0"


def test_version_command_prints_unified_cli_name(capsys):
    assert main(["--version"]) == 0
    assert capsys.readouterr().out.strip() == "ixf 0.1.0"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_cli_contract.py -q`

Expected: FAIL because `ixf_toolbox` does not exist.

- [ ] **Step 3: Implement minimal skeleton**

Create the package, version constant, CLI parser, README, changelog, and project metadata.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_cli_contract.py -q`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: scaffold ixf toolbox"
```

### Task 2: Delegated CLI Surface

**Files:**
- Create: `src/ixf_toolbox/delegate.py`
- Modify: `src/ixf_toolbox/cli.py`
- Test: `tests/test_cli_delegate.py`

**Interfaces:**
- Produces: `ixf_toolbox.delegate.run_command(command: str, args: list[str], runner: Callable[..., object] | None = None) -> int`
- Consumes: `ixf_toolbox.cli.main(argv)`

- [ ] **Step 1: Write failing tests**

```python
from ixf_toolbox.cli import main


def test_docs_read_delegates_to_ixfdoc(monkeypatch):
    calls = []

    def fake_run(command, args):
        calls.append((command, args))
        return 0

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)
    assert main(["docs", "read", "https://tenant.example.test/wiki/example"]) == 0
    assert calls == [("ixfdoc", ["read", "https://tenant.example.test/wiki/example"])]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_cli_delegate.py -q`

Expected: FAIL because delegation is not implemented.

- [ ] **Step 3: Implement command mapping**

Map docs, OKR, cookies, and doctor commands to the legacy engines exactly as specified in the design.

- [ ] **Step 4: Run tests**

Run: `python -m pytest tests/test_cli_contract.py tests/test_cli_delegate.py -q`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src tests
git commit -m "feat: add unified command delegation"
```

### Task 3: Skill Wrapper Installer

**Files:**
- Create: `src/ixf_toolbox/setup.py`
- Create: `skills/codex/ixf-docs-reader/SKILL.md`
- Create: `skills/codex/ixf-docs-writer/SKILL.md`
- Create: `skills/codex/ixf-okr-reader/SKILL.md`
- Create: `skills/codex/ixf-okr-writer/SKILL.md`
- Create: matching `skills/claude-code/*/SKILL.md`
- Modify: `src/ixf_toolbox/cli.py`
- Test: `tests/test_setup.py`

**Interfaces:**
- Produces: `ixf_toolbox.setup.install_skill_wrappers(project_root, home, runtimes, force, env) -> dict[str, object]`
- Produces: `ixf_toolbox.setup.normalize_runtimes(raw: Iterable[str]) -> list[str]`

- [ ] **Step 1: Write failing tests**

```python
from pathlib import Path

from ixf_toolbox.setup import install_skill_wrappers, normalize_runtimes


ROOT = Path(__file__).resolve().parents[1]


def test_normalize_runtimes_supports_aliases():
    assert normalize_runtimes(["auto"]) == ["codex", "claude-code"]
    assert normalize_runtimes(["claude"]) == ["claude-code"]


def test_installs_four_codex_skills(tmp_path):
    result = install_skill_wrappers(ROOT, tmp_path, ["codex"], False, {})
    assert len(result["installed"]) == 4
    assert (tmp_path / ".codex" / "skills" / "ixf-docs-reader" / "SKILL.md").exists()
    assert (tmp_path / ".codex" / "skills" / "ixf-okr-writer" / "SKILL.md").exists()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_setup.py -q`

Expected: FAIL because setup module and skills are not implemented.

- [ ] **Step 3: Implement installer and skill files**

Install four wrappers per selected runtime. Do not overwrite existing skills unless `--force` is passed.

- [ ] **Step 4: Run tests**

Run: `python -m pytest tests/test_setup.py tests/test_cli_delegate.py -q`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src tests skills pyproject.toml
git commit -m "feat: package toolbox skills"
```

### Task 4: Update Command And Release Docs

**Files:**
- Create: `src/ixf_toolbox/update.py`
- Modify: `src/ixf_toolbox/cli.py`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Test: `tests/test_update.py`

**Interfaces:**
- Produces: `ixf_toolbox.update.build_upgrade_command(version: str, repo: str, platform_name: str) -> str`
- Produces: `ixf_toolbox.update.check_latest_release(repo: str, current_version: str, session: Any = requests, platform_name: str | None = None) -> dict[str, object]`

- [ ] **Step 1: Write failing tests**

```python
from ixf_toolbox.update import build_upgrade_command


def test_upgrade_command_targets_toolbox_release_wheel():
    command = build_upgrade_command(
        version="0.1.1",
        repo="serialq7ic4/ixf-toolbox",
        platform_name="macos",
    )
    assert "ixf-toolbox[crypto]" in command
    assert "ixf_toolbox-0.1.1-py3-none-any.whl" in command
    assert "ixf update skills --runtimes auto --json" in command
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_update.py -q`

Expected: FAIL because update module is not implemented.

- [ ] **Step 3: Implement release check and docs**

Follow the writer update contract but target `serialq7ic4/ixf-toolbox`.

- [ ] **Step 4: Run tests and lint**

Run: `python -m pytest -q && python -m ruff check .`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: add toolbox update workflow"
```
