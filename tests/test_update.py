from ixf_toolbox.update import (
    apply_self_update,
    build_upgrade_steps,
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


def test_upgrade_steps_use_safe_argument_vectors():
    steps = build_upgrade_steps(
        version="v0.1.1",
        repo="serialq7ic4/ixf-toolbox",
        platform_name="macos",
        python_executable="/opt/python",
    )

    assert steps == [
        [
            "/opt/python",
            "-m",
            "pip",
            "install",
            "--upgrade",
            "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v0.1.1/ixf_toolbox-0.1.1-py3-none-any.whl",
        ],
        ["ixf", "update", "skills", "--runtimes", "auto", "--json"],
    ]


def test_apply_self_update_dry_run_does_not_run_commands():
    calls = []

    payload = apply_self_update(
        repo="serialq7ic4/ixf-toolbox",
        current_version="0.1.0",
        platform_name="macos",
        apply=False,
        release={"tag_name": "v0.1.1", "html_url": "https://github.example/release"},
        runner=lambda command: calls.append(command) or 0,
        python_executable="/opt/python",
    )

    assert payload["updateAvailable"] is True
    assert payload["applied"] is False
    assert payload["commands"] == []
    assert calls == []


def test_apply_self_update_skips_commands_when_already_current():
    calls = []

    payload = apply_self_update(
        repo="serialq7ic4/ixf-toolbox",
        current_version="0.1.1",
        platform_name="macos",
        apply=True,
        release={"tag_name": "v0.1.1", "html_url": "https://github.example/release"},
        runner=lambda command: calls.append(command) or 0,
        python_executable="/opt/python",
    )

    assert payload["updateAvailable"] is False
    assert payload["applied"] is False
    assert payload["commands"] == []
    assert calls == []


def test_apply_self_update_runs_pip_then_skill_refresh_when_apply_is_set():
    calls = []

    payload = apply_self_update(
        repo="serialq7ic4/ixf-toolbox",
        current_version="0.1.0",
        platform_name="windows",
        apply=True,
        release={"tag_name": "v0.1.1", "html_url": "https://github.example/release"},
        runner=lambda command: calls.append(command) or 0,
        python_executable="/opt/python",
    )

    assert payload["applied"] is True
    assert calls == [
        [
            "/opt/python",
            "-m",
            "pip",
            "install",
            "--upgrade",
            "ixf-toolbox[windows] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v0.1.1/ixf_toolbox-0.1.1-py3-none-any.whl",
        ],
        ["ixf", "update", "skills", "--runtimes", "auto", "--json"],
    ]
    assert payload["commands"] == calls
