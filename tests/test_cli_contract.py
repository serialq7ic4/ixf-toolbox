from ixf_toolbox import __version__
from ixf_toolbox.cli import main


def test_version_constant_is_initial_release():
    assert __version__ == "0.1.0"


def test_version_command_prints_unified_cli_name(capsys):
    assert main(["--version"]) == 0
    assert capsys.readouterr().out.strip() == "ixf 0.1.0"
