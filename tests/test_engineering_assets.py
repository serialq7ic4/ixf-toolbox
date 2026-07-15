import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(relative_path: str) -> str:
    return (ROOT / relative_path).read_text(encoding="utf-8")


def test_ci_workflow_covers_supported_platforms_and_quality_gates():
    text = read(".github/workflows/ci.yml")

    assert "macos-latest" in text
    assert "windows-latest" in text
    assert "actions/setup-go" in text
    assert "go test ./..." in text
    assert "python -m compileall -q src" in text
    assert "python -m pytest -q" in text
    assert "python -m ruff check ." in text
    assert "win32crypt" in text


def test_release_workflow_validates_tag_builds_and_publishes_artifacts():
    text = read(".github/workflows/release.yml")

    assert "tag" in text.lower()
    assert "pyproject.toml" in text
    assert "actions/setup-go" in text
    assert "go test ./..." in text
    assert "Build Go binary artifacts" in text
    assert "ixf_${RELEASE_VERSION}_${goos}_${goarch}" in text
    assert "scripts/smoke-go-binary.sh" in text
    assert "python -m build" in text
    assert "scripts/extract_changelog.py" in text
    assert "softprops/action-gh-release" in text


def test_release_notes_script_extracts_non_empty_changelog_section():
    result = subprocess.run(
        [
            sys.executable,
            "scripts/extract_changelog.py",
            "2.1.0",
            "CHANGELOG.md",
        ],
        cwd=ROOT,
        text=True,
        capture_output=True,
        check=True,
    )

    assert "creating the next Objective" in result.stdout
    assert "stale draft responses" in result.stdout
    assert "## 2.0.0" not in result.stdout


def test_smoke_script_installs_toolbox_wheel_in_isolated_environment():
    text = read("scripts/smoke.sh")

    assert "ixf_toolbox-*.whl" in text
    assert "python -m venv" in text
    assert "--system-site-packages" not in text
    assert "--force-reinstall" in text
    assert '"$venv_ixf" --version' in text
    assert '"$venv_ixf" setup skills --runtimes codex --json' in text
    assert '"$venv_ixf" docs read' in text


def test_go_binary_smoke_script_validates_default_release_artifact():
    text = read("scripts/smoke-go-binary.sh")
    release_doc = read("docs/release.md")

    assert "expected_version" in text
    assert 'ixf "$binary" "$expected_version"' not in text
    assert '"$binary" --version' in text
    assert '"$binary" --help' in text
    assert '"$binary" setup skills --runtimes codex --json' in text
    assert '"$binary" docs read' in text
    assert "scripts/smoke-go-binary.sh" in release_doc


def test_public_project_docs_exist_and_use_toolbox_names():
    for relative_path in [
        "SECURITY.md",
        "PRIVACY.md",
        "CONTRIBUTING.md",
        "README.en.md",
        "docs/release.md",
        "docs/supported-platforms.md",
    ]:
        text = read(relative_path)
        assert "ixf-toolbox" in text or "`ixf`" in text
        assert "ixfdoc" not in text
        assert "ixfwrite" not in text


def test_default_readme_is_full_project_landing_page():
    text = read("README.md")

    for expected in [
        "https://img.shields.io/badge/Python-3.11%2B-3776AB",
        "## 为什么做这个",
        "## 安装到 Codex / Claude Code",
        "## 在 Agent 里使用",
        "## 底层命令",
        "## 手动读取流程",
        "## 手动写入流程",
        "## 支持的能力",
        "## 支持平台",
        "## 迁移",
        "## 隐私与安全",
        "## 开发",
    ]:
        assert expected in text
    assert "ixf docs read" in text
    assert "ixf docs publish" in text
    assert "ixf okr write" in text


def test_v2_docs_make_go_binary_the_default_install_path():
    zh = read("README.md")
    en = read("README.en.md")
    platforms = read("docs/supported-platforms.md")
    migration = read("docs/migration-from-legacy.md")

    assert "Go 二进制" in zh
    assert "默认安装方式" in zh
    assert "Python wheel 保留为 legacy/reference" in zh
    assert "ixf_2.1.0_darwin_arm64" in zh
    assert "v1.x 仍以 Python 版作为默认安装方式" not in zh

    assert "Go binary" in en
    assert "default install path" in en
    assert "Python wheel remains legacy/reference" in en
    assert "ixf_2.1.0_darwin_arm64" in en
    assert "The v1.x line still uses the Python package" not in en

    assert "Go binary" in platforms
    assert "pywin32" not in platforms
    assert "%TEMP%" not in platforms
    assert "$env:TEMP\\ixf_cookies.json" in platforms
    assert "Go binary" in migration


def test_legacy_migration_doc_maps_old_commands_to_toolbox_commands():
    text = read("docs/migration-from-legacy.md")

    assert "`ixfdoc read`" in text
    assert "`ixf docs read`" in text
    assert "`ixfwrite docx publish`" in text
    assert "`ixf docs publish`" in text
    assert "`ixfwrite okr write`" in text
    assert "`ixf okr write`" in text
    assert "does not install `ixfdoc` or `ixfwrite` compatibility shims" in text


def test_issue_and_pr_templates_exist_and_warn_about_sensitive_data():
    paths = [
        ".github/PULL_REQUEST_TEMPLATE.md",
        ".github/ISSUE_TEMPLATE/bug_report.md",
        ".github/ISSUE_TEMPLATE/feature_request.md",
        ".github/ISSUE_TEMPLATE/config.yml",
    ]

    for relative_path in paths:
        text = read(relative_path)
        assert "cookie" in text.lower() or "security" in text.lower()
        assert "ixfdoc" not in text
        assert "ixfwrite" not in text
