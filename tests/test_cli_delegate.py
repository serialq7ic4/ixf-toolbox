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


def test_cookies_export_delegates_to_writer(monkeypatch):
    calls = []

    def fake_run(command, args):
        calls.append((command, args))
        return 0

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run)

    assert main(["cookies", "export", "--provider", "auto"]) == 0
    assert calls == [("ixfwrite", ["cookies", "export", "--provider", "auto"])]
