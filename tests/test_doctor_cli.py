from __future__ import annotations

import json
from pathlib import Path

from ixf_toolbox.cli import main


def test_doctor_json_outputs_toolbox_diagnostics(monkeypatch, capsys):
    def fake_collect_diagnostics(*, home, env, cookies_path):
        assert isinstance(home, Path)
        assert isinstance(env, dict)
        assert cookies_path == Path("/tmp/test-cookies.json")
        return {"ok": True, "version": "2.0.0", "capabilities": {}, "skills": {}, "cookies": {}}

    monkeypatch.setattr("ixf_toolbox.cli.collect_diagnostics", fake_collect_diagnostics)

    assert main(["doctor", "--cookies", "/tmp/test-cookies.json", "--json"]) == 0

    payload = json.loads(capsys.readouterr().out)
    assert payload["ok"] is True
    assert payload["version"] == "2.0.0"


def test_doctor_text_outputs_toolbox_diagnostics(monkeypatch, capsys):
    monkeypatch.setattr(
        "ixf_toolbox.cli.collect_diagnostics",
        lambda *, home, env, cookies_path: {"ok": False, "version": "2.0.0"},
    )
    monkeypatch.setattr(
        "ixf_toolbox.cli.format_diagnostics",
        lambda payload: f"ixf-toolbox {payload['version']}\noverall fail\n",
    )

    assert main(["doctor"]) == 1

    assert capsys.readouterr().out == "ixf-toolbox 2.0.0\noverall fail\n"
