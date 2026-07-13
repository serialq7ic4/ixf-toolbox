from __future__ import annotations

import subprocess

from ixf_toolbox.delegate import rewrite_legacy_output, run_command


def test_rewrite_legacy_output_uses_unified_ixf_commands():
    text = (
        "HINT Run `ixfdoc cookies export --provider auto --output <path>`.\n"
        "Try `ixfdoc read <url> --out-dir <dir>`.\n"
        "Then `ixfwrite okr write --url <url> --input okr.json --apply`.\n"
    )

    rewritten = rewrite_legacy_output(text)

    assert "ixf cookies export --provider auto --output <path>" in rewritten
    assert "ixf docs read <url> --out-dir <dir>" in rewritten
    assert "ixf okr write --url <url> --input okr.json --apply" in rewritten
    assert "ixfdoc" not in rewritten
    assert "ixfwrite" not in rewritten


def test_run_command_rewrites_legacy_stdout_and_stderr(capsys):
    def fake_runner(command, **kwargs):
        assert command == ["ixfdoc", "read", "https://tenant.example.test/wiki/example"]
        assert kwargs["check"] is False
        assert kwargs["capture_output"] is True
        assert kwargs["text"] is True
        return subprocess.CompletedProcess(
            command,
            5,
            stdout="ERROR see `ixfdoc read <url>`\n",
            stderr="HINT run `ixfdoc cookies export --provider auto`\n",
        )

    assert run_command(
        "ixfdoc",
        ["read", "https://tenant.example.test/wiki/example"],
        runner=fake_runner,
    ) == 5

    captured = capsys.readouterr()
    assert "ixf docs read <url>" in captured.out
    assert "ixf cookies export --provider auto" in captured.err
    assert "ixfdoc" not in captured.out
    assert "ixfdoc" not in captured.err
