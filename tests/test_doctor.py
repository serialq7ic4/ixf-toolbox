from __future__ import annotations

import json
import subprocess

from ixf_toolbox.doctor import collect_diagnostics, format_diagnostics


def test_collect_diagnostics_reports_engines_skills_and_cookie_metadata(tmp_path):
    cookies = tmp_path / "cookies.json"
    cookies.write_text(
        json.dumps(
            [
                {"name": "_csrf_token", "value": "dummy-csrf"},
                {"name": "session", "value": "dummy-session"},
            ]
        ),
        encoding="utf-8",
    )
    codex_dir = tmp_path / "codex-skills"
    for skill_name in ["ixf-docs-reader", "ixf-docs-writer", "ixf-okr-reader", "ixf-okr-writer"]:
        skill_dir = codex_dir / skill_name
        skill_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text(f"name: {skill_name}\n", encoding="utf-8")

    def executable_lookup(name: str) -> str | None:
        return f"/usr/local/bin/{name}"

    def command_runner(command, **kwargs):
        assert kwargs["check"] is False
        assert kwargs["capture_output"] is True
        assert kwargs["text"] is True
        return subprocess.CompletedProcess(command, 0, stdout=f"{command[0]} 1.2.3\n", stderr="")

    payload = collect_diagnostics(
        home=tmp_path,
        env={"IXF_TOOLBOX_CODEX_SKILLS_DIR": str(codex_dir)},
        cookies_path=cookies,
        executable_lookup=executable_lookup,
        command_runner=command_runner,
    )

    assert payload["ok"] is True
    assert payload["version"] == "0.4.0"
    assert payload["engines"]["ixfdoc"]["ok"] is True
    assert payload["engines"]["ixfdoc"]["path"] == "/usr/local/bin/ixfdoc"
    assert payload["engines"]["ixfwrite"]["version"] == "ixfwrite 1.2.3"
    assert payload["skills"]["codex"]["ok"] is True
    assert payload["skills"]["codex"]["installed"]["ixf-okr-writer"] is True
    assert payload["cookies"]["ok"] is True
    assert payload["cookies"]["cookieCount"] == 2
    assert payload["cookies"]["hasCsrf"] is True
    assert "dummy-csrf" not in json.dumps(payload)
    assert "dummy-session" not in json.dumps(payload)


def test_collect_diagnostics_marks_missing_engines_and_skills_unhealthy(tmp_path):
    payload = collect_diagnostics(
        home=tmp_path,
        env={},
        cookies_path=tmp_path / "missing-cookies.json",
        executable_lookup=lambda name: None,
        command_runner=lambda command, **kwargs: subprocess.CompletedProcess(command, 1),
    )

    assert payload["ok"] is False
    assert payload["engines"]["ixfdoc"]["ok"] is False
    assert payload["engines"]["ixfwrite"]["ok"] is False
    assert payload["skills"]["codex"]["ok"] is False
    assert payload["cookies"]["exists"] is False


def test_collect_diagnostics_reports_invalid_cookie_file_without_crashing(tmp_path):
    cookies = tmp_path / "cookies.json"
    cookies.write_text("{not-json", encoding="utf-8")

    payload = collect_diagnostics(
        home=tmp_path,
        env={},
        cookies_path=cookies,
        executable_lookup=lambda name: f"/bin/{name}",
        command_runner=lambda command, **kwargs: subprocess.CompletedProcess(command, 0),
    )

    assert payload["ok"] is False
    assert payload["cookies"]["ok"] is False
    assert payload["cookies"]["exists"] is True
    assert payload["cookies"]["cookieCount"] == 0
    assert "error" in payload["cookies"]


def test_collect_diagnostics_uses_toolbox_cookie_core(monkeypatch, tmp_path):
    calls = []

    def fake_cookie_diagnostics(path):
        calls.append(path)
        return {
            "ok": True,
            "exists": True,
            "path": str(path),
            "cookieCount": 1,
            "cookieNames": ["_csrf_token"],
            "hasCsrf": True,
            "hasLgwCsrf": False,
        }

    monkeypatch.setattr("ixf_toolbox.doctor.cookie_diagnostics", fake_cookie_diagnostics)
    cookies = tmp_path / "cookies.json"

    payload = collect_diagnostics(
        home=tmp_path,
        env={},
        cookies_path=cookies,
        executable_lookup=lambda name: f"/bin/{name}",
        command_runner=lambda command, **kwargs: subprocess.CompletedProcess(command, 0),
    )

    assert calls == [cookies]
    assert payload["cookies"]["ok"] is True


def test_format_diagnostics_is_human_readable_without_secret_values(tmp_path):
    payload = {
        "ok": False,
        "version": "0.4.0",
        "engines": {
            "ixfdoc": {"ok": True, "path": "/bin/ixfdoc", "version": "ixfdoc 1.0"},
            "ixfwrite": {"ok": False, "path": "", "version": ""},
        },
        "skills": {
            "codex": {
                "ok": True,
                "dir": str(tmp_path / "skills"),
                "installed": {"ixf-docs-reader": True},
            }
        },
        "cookies": {
            "ok": True,
            "exists": True,
            "path": str(tmp_path / "cookies.json"),
            "cookieCount": 1,
            "hasCsrf": True,
            "hasLgwCsrf": False,
            "cookieNames": ["_csrf_token"],
        },
    }

    text = format_diagnostics(payload)

    assert "ixf-toolbox 0.4.0" in text
    assert "engine ixfdoc ok" in text
    assert "engine ixfwrite missing" in text
    assert "cookies ok count=1 csrf=true lgw_csrf=false" in text
    assert "_csrf_token" not in text
