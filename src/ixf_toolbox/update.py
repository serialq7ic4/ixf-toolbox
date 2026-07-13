from __future__ import annotations

import platform
import re
from typing import Any

import requests


DEFAULT_RELEASE_REPO = "serialq7ic4/ixf-toolbox"


def normalize_version(value: str) -> tuple[int, int, int]:
    normalized = value.strip().removeprefix("v").removeprefix("V")
    match = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)", normalized)
    if match is None:
        raise ValueError(f"invalid semantic version: {value}")
    return tuple(int(part) for part in match.groups())


def version_string(value: str) -> str:
    return ".".join(str(part) for part in normalize_version(value))


def is_newer_version(current: str, candidate: str) -> bool:
    return normalize_version(candidate) > normalize_version(current)


def current_platform_name() -> str:
    system = platform.system().lower()
    if system == "darwin":
        return "macos"
    if system == "windows":
        return "windows"
    return "other"


def build_upgrade_command(*, version: str, repo: str, platform_name: str) -> str:
    normalized = version_string(version)
    extra = "windows" if platform_name == "windows" else "crypto"
    wheel = f"ixf_toolbox-{normalized}-py3-none-any.whl"
    return (
        "python -m pip install --upgrade "
        f"\"ixf-toolbox[{extra}] @ "
        f"https://github.com/{repo}/releases/download/v{normalized}/{wheel}\" "
        "&& ixf update skills --runtimes auto --json"
    )


def get_latest_github_release(repo: str, *, session: Any = requests) -> dict[str, object]:
    response = session.get(
        f"https://api.github.com/repos/{repo}/releases/latest",
        headers={"Accept": "application/vnd.github+json"},
        timeout=15,
    )
    response.raise_for_status()
    payload = response.json()
    if not isinstance(payload, dict):
        raise RuntimeError("GitHub release response was not a JSON object.")
    return payload


def check_latest_release(
    *,
    repo: str,
    current_version: str,
    session: Any = requests,
    platform_name: str | None = None,
) -> dict[str, object]:
    release = get_latest_github_release(repo, session=session)
    latest_tag = str(release.get("tag_name") or "").strip()
    if not latest_tag:
        raise RuntimeError("GitHub release response did not include tag_name.")
    latest_version = version_string(latest_tag)
    update_available = is_newer_version(current_version, latest_version)
    selected_platform = platform_name or current_platform_name()
    return {
        "ok": True,
        "currentVersion": version_string(current_version),
        "latestVersion": latest_version,
        "latestTag": latest_tag,
        "updateAvailable": update_available,
        "releaseUrl": str(release.get("html_url") or ""),
        "installCommand": (
            build_upgrade_command(
                version=latest_version,
                repo=repo,
                platform_name=selected_platform,
            )
            if update_available
            else ""
        ),
    }
