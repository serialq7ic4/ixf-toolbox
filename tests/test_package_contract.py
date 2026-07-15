from __future__ import annotations

from pathlib import Path
import tomllib


ROOT = Path(__file__).resolve().parents[1]


def load_pyproject():
    return tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))


def test_pyproject_no_longer_declares_python_package_metadata():
    pyproject = load_pyproject()

    assert "build-system" not in pyproject
    assert "project" not in pyproject
    assert "tool" in pyproject


def test_pyproject_keeps_only_test_tool_configuration():
    pyproject = load_pyproject()
    tool = pyproject["tool"]

    assert tool["pytest"]["ini_options"]["testpaths"] == ["tests"]
    assert tool["ruff"]["line-length"] == 100
    assert tool["ruff"]["target-version"] == "py311"


def test_python_runtime_package_has_been_removed():
    assert not (ROOT / "src" / "ixf_toolbox").exists()
