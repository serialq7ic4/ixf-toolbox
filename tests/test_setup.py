from pathlib import Path

from ixf_toolbox.setup import install_skill_wrappers, normalize_runtimes


ROOT = Path(__file__).resolve().parents[1]


def test_normalize_runtimes_supports_auto_and_aliases():
    assert normalize_runtimes(["auto"]) == ["codex", "claude-code"]
    assert normalize_runtimes(["all"]) == ["codex", "claude-code"]
    assert normalize_runtimes(["codex", "claude"]) == ["codex", "claude-code"]
    assert normalize_runtimes(["claude_code"]) == ["claude-code"]
    assert normalize_runtimes(["none"]) == []


def test_install_skill_wrapper_installs_four_codex_skills(tmp_path):
    result = install_skill_wrappers(
        project_root=ROOT,
        home=tmp_path,
        runtimes=["codex"],
        force=False,
        env={},
    )

    assert len(result["installed"]) == 4
    assert result["skipped"] == []
    for name in ["ixf-docs-reader", "ixf-docs-writer", "ixf-okr-reader", "ixf-okr-writer"]:
        skill = tmp_path / ".codex" / "skills" / name / "SKILL.md"
        assert skill.exists()
        assert f"name: {name}" in skill.read_text(encoding="utf-8")


def test_okr_writer_skill_documents_api_only_ixf_command():
    for runtime in ["codex", "claude-code"]:
        skill = ROOT / "skills" / runtime / "ixf-okr-writer" / "SKILL.md"
        text = skill.read_text(encoding="utf-8")

        assert "API-only" in text
        assert "ixf okr write" in text
        assert "ixfwrite" not in text
        assert "playwright" not in text.lower()


def test_docs_writer_skill_documents_api_only_ixf_command():
    for runtime in ["codex", "claude-code"]:
        skill = ROOT / "skills" / runtime / "ixf-docs-writer" / "SKILL.md"
        text = skill.read_text(encoding="utf-8")

        assert "API-only" in text
        assert "ixf docs publish" in text
        assert "ixfwrite" not in text
        assert "playwright" not in text.lower()


def test_reader_skills_do_not_export_cookies_to_unused_custom_path():
    for runtime in ["codex", "claude-code"]:
        for skill_name in ["ixf-docs-reader", "ixf-okr-reader"]:
            skill = ROOT / "skills" / runtime / skill_name / "SKILL.md"
            text = skill.read_text(encoding="utf-8")

            assert "ixf cookies export --provider auto" in text
            assert "--output ~/.ixf/cookies.json" not in text


def test_okr_reader_skill_documents_current_cli_shape():
    for runtime in ["codex", "claude-code"]:
        skill = ROOT / "skills" / runtime / "ixf-okr-reader" / "SKILL.md"
        text = skill.read_text(encoding="utf-8")

        assert 'ixf okr read "<okr-url>"' in text
        assert "--out-dir" not in text
        assert "--print-manifest" not in text


def test_install_skill_wrapper_does_not_overwrite_without_force(tmp_path):
    destination = tmp_path / ".codex" / "skills" / "ixf-docs-reader"
    destination.mkdir(parents=True)
    marker = destination / "marker.txt"
    marker.write_text("keep", encoding="utf-8")

    result = install_skill_wrappers(
        project_root=ROOT,
        home=tmp_path,
        runtimes=["codex"],
        force=False,
        env={},
    )

    assert marker.read_text(encoding="utf-8") == "keep"
    assert any(item["reason"] == "exists" for item in result["skipped"])


def test_install_skill_wrapper_respects_env_override(tmp_path):
    custom_dir = tmp_path / "custom-codex"
    result = install_skill_wrappers(
        project_root=ROOT,
        home=tmp_path,
        runtimes=["codex"],
        force=False,
        env={"IXF_TOOLBOX_CODEX_SKILLS_DIR": str(custom_dir)},
    )

    assert len(result["installed"]) == 4
    assert (custom_dir / "ixf-docs-reader" / "SKILL.md").exists()
