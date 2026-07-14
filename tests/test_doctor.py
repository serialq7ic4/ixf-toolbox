from __future__ import annotations

import json

from ixf_toolbox.doctor import collect_diagnostics, format_diagnostics


SKILL_NAMES = [
    "using-ixf-toolbox",
    "ixf-docs-reader",
    "ixf-docs-writer",
    "ixf-okr-reader",
    "ixf-okr-writer",
]


def install_codex_skills(tmp_path):
    codex_dir = tmp_path / "codex-skills"
    for skill_name in SKILL_NAMES:
        skill_dir = codex_dir / skill_name
        skill_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text(f"name: {skill_name}\n", encoding="utf-8")
    return codex_dir


def write_valid_cookies(path):
    path.write_text(
        json.dumps(
            [
                {"name": "_csrf_token", "value": "dummy-csrf"},
                {"name": "session", "value": "dummy-session"},
            ]
        ),
        encoding="utf-8",
    )


def test_collect_diagnostics_reports_native_capabilities_skills_and_cookie_metadata(tmp_path):
    cookies = tmp_path / "cookies.json"
    write_valid_cookies(cookies)
    codex_dir = install_codex_skills(tmp_path)

    payload = collect_diagnostics(
        home=tmp_path,
        env={"IXF_TOOLBOX_CODEX_SKILLS_DIR": str(codex_dir)},
        cookies_path=cookies,
    )

    assert payload["ok"] is True
    assert payload["version"] == "1.7.0"
    assert "engines" not in payload
    assert payload["capabilities"] == {
        "docsRead": True,
        "docsPublish": True,
        "okrRead": True,
        "okrWrite": True,
        "cookiesExport": True,
    }
    assert payload["skills"]["codex"]["ok"] is True
    assert payload["skills"]["codex"]["installed"]["ixf-okr-writer"] is True
    assert payload["cookies"]["ok"] is True
    assert payload["cookies"]["cookieCount"] == 2
    assert payload["cookies"]["hasCsrf"] is True
    assert "dummy-csrf" not in json.dumps(payload)
    assert "dummy-session" not in json.dumps(payload)


def test_collect_diagnostics_does_not_require_legacy_engines(tmp_path):
    cookies = tmp_path / "cookies.json"
    write_valid_cookies(cookies)
    codex_dir = install_codex_skills(tmp_path)

    payload = collect_diagnostics(
        home=tmp_path,
        env={"IXF_TOOLBOX_CODEX_SKILLS_DIR": str(codex_dir)},
        cookies_path=cookies,
    )

    assert payload["ok"] is True
    assert "engines" not in payload


def test_collect_diagnostics_marks_missing_skills_and_cookies_unhealthy(tmp_path):
    payload = collect_diagnostics(
        home=tmp_path,
        env={},
        cookies_path=tmp_path / "missing-cookies.json",
    )

    assert payload["ok"] is False
    assert payload["skills"]["codex"]["ok"] is False
    assert payload["cookies"]["exists"] is False


def test_collect_diagnostics_reports_invalid_cookie_file_without_crashing(tmp_path):
    cookies = tmp_path / "cookies.json"
    cookies.write_text("{not-json", encoding="utf-8")

    payload = collect_diagnostics(
        home=tmp_path,
        env={},
        cookies_path=cookies,
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
    )

    assert calls == [cookies]
    assert payload["cookies"]["ok"] is True


def test_format_diagnostics_is_human_readable_without_secret_values(tmp_path):
    payload = {
        "ok": False,
        "version": "1.7.0",
        "capabilities": {
            "docsRead": True,
            "docsPublish": True,
            "okrRead": True,
            "okrWrite": True,
            "cookiesExport": True,
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

    assert "ixf-toolbox 1.7.0" in text
    assert "native docsRead=true docsPublish=true okrRead=true okrWrite=true cookiesExport=true" in text
    assert "engine ixfdoc" not in text
    assert "engine ixfwrite" not in text
    assert "cookies ok count=1 csrf=true lgw_csrf=false" in text
    assert "_csrf_token" not in text
