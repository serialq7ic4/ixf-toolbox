from __future__ import annotations

import json
from collections.abc import Mapping
from pathlib import Path

from ixf_toolbox import __version__
from ixf_toolbox.core.cookies import cookie_diagnostics
from ixf_toolbox.setup import SKILL_NAMES, detect_runtime_targets


NATIVE_CAPABILITIES = {
    "docsRead": True,
    "docsPublish": True,
    "okrRead": True,
    "okrWrite": True,
    "cookiesExport": True,
}


def _load_cookie_diagnostics(cookie_path: Path) -> dict[str, object]:
    path = cookie_path.expanduser()
    return cookie_diagnostics(path)


def _skills_status(home: Path, env: Mapping[str, str]) -> dict[str, object]:
    result: dict[str, object] = {}
    for target in detect_runtime_targets(home, env):
        installed = {
            skill_name: (target.skills_dir / skill_name / "SKILL.md").exists()
            for skill_name in SKILL_NAMES
        }
        result[target.key] = {
            "ok": all(installed.values()),
            "dir": str(target.skills_dir),
            "installed": installed,
        }
    return result


def collect_diagnostics(
    *,
    home: Path,
    env: Mapping[str, str],
    cookies_path: Path,
) -> dict[str, object]:
    skills = _skills_status(home, env)
    cookies = _load_cookie_diagnostics(cookies_path)
    skills_ok = any(bool(status["ok"]) for status in skills.values())
    cookies_ok = bool(cookies.get("ok"))
    return {
        "ok": skills_ok and cookies_ok,
        "version": __version__,
        "capabilities": dict(NATIVE_CAPABILITIES),
        "skills": skills,
        "cookies": cookies,
    }


def _bool_text(value: object) -> str:
    return "true" if bool(value) else "false"


def format_diagnostics(payload: Mapping[str, object]) -> str:
    lines = [
        f"ixf-toolbox {payload.get('version', '')}",
        f"overall {'ok' if payload.get('ok') else 'fail'}",
    ]
    capabilities = payload.get("capabilities", {})
    if isinstance(capabilities, Mapping):
        details = " ".join(
            f"{name}={_bool_text(capabilities.get(name))}"
            for name in NATIVE_CAPABILITIES
        )
        lines.append(f"native {details}")
    skills = payload.get("skills", {})
    if isinstance(skills, Mapping):
        for runtime, status in skills.items():
            if not isinstance(status, Mapping):
                continue
            state = "ok" if status.get("ok") else "incomplete"
            installed = status.get("installed", {})
            installed_count = (
                sum(1 for value in installed.values() if value)
                if isinstance(installed, Mapping)
                else 0
            )
            lines.append(f"skills {runtime} {state} installed={installed_count}/{len(SKILL_NAMES)}")
    cookies = payload.get("cookies", {})
    if isinstance(cookies, Mapping):
        state = "ok" if cookies.get("ok") else "missing"
        lines.append(
            "cookies "
            f"{state} count={cookies.get('cookieCount', 0)} "
            f"csrf={_bool_text(cookies.get('hasCsrf'))} "
            f"lgw_csrf={_bool_text(cookies.get('hasLgwCsrf'))}"
        )
        path = cookies.get("path")
        if path:
            lines.append(f"cookies path {path}")
    return "\n".join(lines) + "\n"


def to_json(payload: Mapping[str, object]) -> str:
    return json.dumps(payload, ensure_ascii=False)
