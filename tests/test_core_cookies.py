from __future__ import annotations

import json
import os
import platform
import stat
import subprocess

from ixf_toolbox.core.cookies import (
    DEFAULT_COOKIES,
    cookie_diagnostics,
    export_cookie_session,
    load_cookie_objects,
    write_cookie_json,
)


def assert_private_posix_mode(path):
    if os.name != "nt":
        assert stat.S_IMODE(path.stat().st_mode) == 0o600


def test_cookie_json_round_trip_is_private_and_secret_safe(tmp_path):
    output = tmp_path / "cookies.json"

    write_cookie_json(
        output,
        [
            {"name": "_csrf_token", "value": "dummy-csrf", "domain": ".example.test"},
            {"name": "session", "value": "dummy-session", "domain": ".example.test"},
        ],
    )

    payload = cookie_diagnostics(output)
    serialized = json.dumps(payload, ensure_ascii=False)

    assert DEFAULT_COOKIES == "/tmp/ixunfei_profile_explorer_cookies.json"
    assert_private_posix_mode(output)
    assert load_cookie_objects(output)[0]["name"] == "_csrf_token"
    assert payload["exists"] is True
    assert payload["cookieCount"] == 2
    assert payload["hasCsrf"] is True
    assert payload["cookieNames"] == ["_csrf_token", "session"]
    assert "dummy-csrf" not in serialized
    assert "dummy-session" not in serialized


def test_cookie_diagnostics_reports_invalid_cookie_file_without_crashing(tmp_path):
    cookies = tmp_path / "cookies.json"
    cookies.write_text("{not-json", encoding="utf-8")

    payload = cookie_diagnostics(cookies)

    assert payload["ok"] is False
    assert payload["exists"] is True
    assert payload["cookieCount"] == 0
    assert payload["hasCsrf"] is False
    assert "error" in payload


def test_export_cookie_session_auto_routes_by_platform(monkeypatch, tmp_path):
    calls = []

    def fake_macos_export(**kwargs):
        calls.append(("macos", kwargs))
        return {"ok": True, "provider": "macos-larkshell", "output": str(kwargs["output"])}

    def fake_windows_export(**kwargs):
        calls.append(("windows", kwargs))
        return {"ok": True, "provider": "windows-larkshell", "output": str(kwargs["output"])}

    monkeypatch.setattr(platform, "system", lambda: "Darwin")
    monkeypatch.setattr("ixf_toolbox.core.cookies.macos_larkshell.export_cookies", fake_macos_export)
    monkeypatch.setattr("ixf_toolbox.core.cookies.windows_larkshell.export_cookies", fake_windows_export)

    payload = export_cookie_session(provider="auto", output=tmp_path / "cookies.json")

    assert payload["provider"] == "macos-larkshell"
    assert calls[0][0] == "macos"

    monkeypatch.setattr(platform, "system", lambda: "Windows")
    payload = export_cookie_session(provider="auto", output=tmp_path / "cookies-win.json")

    assert payload["provider"] == "windows-larkshell"
    assert calls[1][0] == "windows"


def test_macos_keychain_lookup_does_not_require_account(monkeypatch):
    from ixf_toolbox.core.cookies.macos_larkshell import find_keychain_password

    calls = []

    def fake_check_output(command, **_kwargs):
        calls.append(command)
        return "dummy-password\n"

    monkeypatch.setattr(subprocess, "check_output", fake_check_output)

    assert find_keychain_password("Suite App Safe Storage") == "dummy-password"
    assert "-a" not in calls[0]
