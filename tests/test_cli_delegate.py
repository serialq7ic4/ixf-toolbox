from ixf_toolbox.cli import main


def test_docs_read_uses_toolbox_core(monkeypatch, tmp_path, capsys):
    def fake_run(command, args):
        raise AssertionError("ixf docs read must not delegate to ixfdoc")

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run, raising=False)
    source = tmp_path / "source.md"
    source.write_text("# Source\n\nNative docs read.\n", encoding="utf-8")

    assert main(["docs", "read", str(source)]) == 0
    assert capsys.readouterr().out == "# Source\n\nNative docs read.\n"


def test_docs_publish_uses_toolbox_core(monkeypatch, tmp_path, capsys):
    calls = []

    def fake_run(command, args):
        raise AssertionError("ixf docs publish must not delegate to ixfwrite")

    def fake_publish_markdown(config):
        calls.append(config)
        return {
            "ok": True,
            "dryRun": True,
            "title": "Native Publish",
            "counts": {"text": 1},
        }

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run, raising=False)
    monkeypatch.setattr("ixf_toolbox.cli.publish_markdown", fake_publish_markdown, raising=False)
    source = tmp_path / "notes.md"
    source.write_text("# Native Publish\n\nBody.\n", encoding="utf-8")

    assert main(
        [
            "docs",
            "publish",
            str(source),
            "--base-url",
            "https://tenant.example.test",
            "--title-suffix",
            " - Draft",
            "--require",
            "Body",
            "--dry-run",
        ]
    ) == 0
    assert calls[0].markdown_path == source
    assert calls[0].base_url == "https://tenant.example.test"
    assert calls[0].title_suffix == " - Draft"
    assert calls[0].required_text == ("Body",)
    assert calls[0].apply is False
    assert '"dryRun": true' in capsys.readouterr().out


def test_okr_read_uses_toolbox_core(monkeypatch, capsys):
    def fake_run(command, args):
        raise AssertionError("ixf okr read must not delegate to ixfdoc")

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run, raising=False)
    monkeypatch.setattr(
        "ixf_toolbox.cli.read_okr_url",
        lambda source, **kwargs: {
            "source": source,
            "kind": "okr",
            "title": "OKR - Fixture Owner",
            "token": "okr-fixture-200",
            "content": "# OKR - Fixture Owner\n\n- KR1: Native read\n",
            "counts": {"objectives": 1, "key_results": 1},
        },
    )

    assert main(["okr", "read", "https://tenant.example.test/okr/user/example"]) == 0
    assert capsys.readouterr().out == "# OKR - Fixture Owner\n\n- KR1: Native read\n"


def test_okr_write_uses_toolbox_core(monkeypatch, capsys):
    calls = []

    def fake_run(command, args):
        raise AssertionError("ixf okr write must not delegate to ixfwrite")

    def fake_write_okr(config):
        calls.append(config)
        return {
            "ok": True,
            "dryRun": True,
            "actions": ["target objective index: O3"],
            "objectiveCount": 2,
        }

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run, raising=False)
    monkeypatch.setattr("ixf_toolbox.cli.write_okr", fake_write_okr, raising=False)

    assert main(
        [
            "okr",
            "write",
            "--url",
            "https://tenant.example.test/okr/user/example/?okrId=example-okr",
            "--input",
            "okr.json",
            "--objective-index",
            "3",
        ]
    ) == 0
    assert calls[0].url == "https://tenant.example.test/okr/user/example/?okrId=example-okr"
    assert str(calls[0].input_path) == "okr.json"
    assert calls[0].objective_index == 3
    assert calls[0].apply is False
    assert '"dryRun": true' in capsys.readouterr().out


def test_cookies_export_uses_toolbox_core(monkeypatch, tmp_path, capsys):
    calls = []

    def fake_run(command, args):
        raise AssertionError("ixf cookies export must not delegate to ixfwrite")

    monkeypatch.setattr("ixf_toolbox.cli.run_command", fake_run, raising=False)
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
