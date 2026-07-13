from ixf_toolbox.cli import main


def test_top_level_help_lists_commands(capsys):
    assert main(["--help"]) == 0

    output = capsys.readouterr().out
    assert "usage: ixf" in output
    assert "docs" in output
    assert "okr" in output
    assert "update" in output


def test_docs_help_lists_supported_subcommands(capsys):
    assert main(["docs", "--help"]) == 0

    output = capsys.readouterr().out
    assert "usage: ixf docs" in output
    assert "read" in output
    assert "publish" in output
    assert "inspect" in output


def test_docs_without_subcommand_prints_help_and_returns_usage_error(capsys):
    assert main(["docs"]) == 2

    captured = capsys.readouterr()
    assert "usage: ixf docs" in captured.err
    assert "read" in captured.err
    assert "publish" in captured.err


def test_okr_help_lists_supported_subcommands(capsys):
    assert main(["okr", "--help"]) == 0

    output = capsys.readouterr().out
    assert "usage: ixf okr" in output
    assert "read" in output
    assert "write" in output


def test_okr_without_subcommand_prints_help_and_returns_usage_error(capsys):
    assert main(["okr"]) == 2

    captured = capsys.readouterr()
    assert "usage: ixf okr" in captured.err
    assert "read" in captured.err
    assert "write" in captured.err
