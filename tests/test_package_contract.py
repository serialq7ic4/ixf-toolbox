from __future__ import annotations

from pathlib import Path
import tomllib


ROOT = Path(__file__).resolve().parents[1]


def load_pyproject():
    return tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))


def test_runtime_dependencies_do_not_install_legacy_reader_or_writer():
    pyproject = load_pyproject()
    dependencies = pyproject["project"]["dependencies"]
    joined = "\n".join(dependencies)

    assert dependencies == ["requests>=2.31"]
    assert "ixunfei-docx-reader" not in joined
    assert "ixunfei-docx-writer" not in joined
    assert "ixfdoc" not in joined
    assert "ixfwrite" not in joined


def test_crypto_and_windows_extras_are_toolbox_owned():
    optional = load_pyproject()["project"]["optional-dependencies"]

    assert optional["crypto"] == ["cryptography>=42.0"]
    assert optional["windows"] == ["cryptography>=42.0", "pywin32>=306"]


def test_delegate_bridge_has_been_removed():
    assert not (ROOT / "src" / "ixf_toolbox" / "delegate.py").exists()
