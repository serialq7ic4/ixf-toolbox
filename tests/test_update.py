from ixf_toolbox.update import (
    build_upgrade_command,
    is_newer_version,
    normalize_version,
)


def test_normalize_version_accepts_release_tags():
    assert normalize_version("v0.1.0") == (0, 1, 0)
    assert normalize_version("0.2.3") == (0, 2, 3)


def test_is_newer_version_compares_semantic_versions():
    assert is_newer_version("0.1.0", "v0.1.1") is True
    assert is_newer_version("0.1.1", "v0.1.1") is False


def test_upgrade_command_targets_toolbox_release_wheel():
    command = build_upgrade_command(
        version="0.1.1",
        repo="serialq7ic4/ixf-toolbox",
        platform_name="macos",
    )

    assert "ixf-toolbox[crypto]" in command
    assert "/releases/download/v0.1.1/" in command
    assert "ixf_toolbox-0.1.1-py3-none-any.whl" in command
    assert "ixf update skills --runtimes auto --json" in command
