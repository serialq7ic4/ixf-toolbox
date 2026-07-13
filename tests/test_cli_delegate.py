from ixf_toolbox.cli import main


def test_docs_read_delegates_to_ixfdoc(monkeypatch):
    calls = []

    def fake_run(command, args):
        calls.append((command, args))
        return 0

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)

    assert main(["docs", "read", "https://tenant.example.test/wiki/example"]) == 0
    assert calls == [("ixfdoc", ["read", "https://tenant.example.test/wiki/example"])]


def test_docs_publish_delegates_to_ixfwrite_docx_publish(monkeypatch):
    calls = []

    def fake_run(command, args):
        calls.append((command, args))
        return 0

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)

    assert main(["docs", "publish", "notes.md", "--dry-run"]) == 0
    assert calls == [("ixfwrite", ["docx", "publish", "notes.md", "--dry-run"])]


def test_okr_read_delegates_to_ixfdoc_read(monkeypatch):
    calls = []

    def fake_run(command, args):
        calls.append((command, args))
        return 0

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)

    assert main(["okr", "read", "https://tenant.example.test/okr/user/example"]) == 0
    assert calls == [("ixfdoc", ["read", "https://tenant.example.test/okr/user/example"])]


def test_okr_write_delegates_to_ixfwrite_okr_write(monkeypatch):
    calls = []

    def fake_run(command, args):
        calls.append((command, args))
        return 0

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)

    assert main(["okr", "write", "--url", "https://tenant.example.test/okr", "--input", "okr.json"]) == 0
    assert calls == [
        ("ixfwrite", ["okr", "write", "--url", "https://tenant.example.test/okr", "--input", "okr.json"])
    ]


def test_cookies_export_uses_toolbox_core(monkeypatch, tmp_path, capsys):
    calls = []

    def fake_run(command, args):
        raise AssertionError("ixf cookies export must not delegate to ixfwrite")

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)
    output = tmp_path / "cookies.json"

    def fake_export_cookie_session(**kwargs):
        calls.append(kwargs)
        return {
            "ok": True,
            "provider": kwargs["provider"],
            "cookieCount": 1,
            "hasCsrf": True,
            "output": str(kwargs["output"]),
        }

    monkeypatch.setattr("ixf_toolbox.cli.export_cookie_session", fake_export_cookie_session)

    assert main(["cookies", "export", "--provider", "auto", "--output", str(output)]) == 0
    assert calls[0]["provider"] == "auto"
    assert calls[0]["output"] == output
    assert calls[0]["cookies_db"] is None
    assert calls[0]["app_data"] is None
    assert '"provider": "auto"' in capsys.readouterr().out
