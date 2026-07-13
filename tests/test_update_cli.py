import json

from ixf_toolbox.cli import main


def test_update_self_json_defaults_to_dry_run(monkeypatch, capsys):
    calls = []

    def fake_apply_self_update(**kwargs):
        calls.append(kwargs)
        return {
            "ok": True,
            "currentVersion": "0.8.0",
            "latestVersion": "0.9.0",
            "latestTag": "v0.9.0",
            "updateAvailable": True,
            "releaseUrl": "https://github.example/release",
            "installCommand": "python -m pip install --upgrade example",
            "applied": False,
            "commands": [],
        }

    monkeypatch.setattr("ixf_toolbox.cli.apply_self_update", fake_apply_self_update)

    assert main(["update", "self", "--json"]) == 0

    assert calls[0]["apply"] is False
    payload = json.loads(capsys.readouterr().out)
    assert payload["updateAvailable"] is True
    assert payload["applied"] is False


def test_update_self_apply_passes_apply_flag(monkeypatch, capsys):
    calls = []

    def fake_apply_self_update(**kwargs):
        calls.append(kwargs)
        return {
            "ok": True,
            "currentVersion": "0.8.0",
            "latestVersion": "0.9.0",
            "latestTag": "v0.9.0",
            "updateAvailable": True,
            "releaseUrl": "https://github.example/release",
            "installCommand": "python -m pip install --upgrade example",
            "applied": True,
            "commands": [["python", "-m", "pip", "install", "--upgrade", "example"]],
        }

    monkeypatch.setattr("ixf_toolbox.cli.apply_self_update", fake_apply_self_update)

    assert main(["update", "self", "--apply", "--json"]) == 0

    assert calls[0]["apply"] is True
    payload = json.loads(capsys.readouterr().out)
    assert payload["applied"] is True
    assert payload["commands"][0][0] == "python"
