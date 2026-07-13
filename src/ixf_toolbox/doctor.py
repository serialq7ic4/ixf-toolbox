from __future__ import annotations

import json
import shutil
import subprocess
from collections.abc import Callable, Mapping
from pathlib import Path
from typing import Any

from ixf_toolbox import __version__
from ixf_toolbox.setup import SKILL_NAMES, detect_runtime_targets


DEFAULT_COOKIES = "/tmp/ixunfei_profile_explorer_cookies.json"
ExecutableLookup = Callable[[str], str | None]
CommandRunner = Callable[..., Any]


def _load_cookie_diagnostics(cookie_path: Path) -> dict[str, object]:
    try:
        from ixunfei_docx_writer.cookies.common import cookie_diagnostics
    except ImportError:
        path = cookie_path.expanduser()
        return {
            "ok": False,
            "exists": path.exists(),
            "path": str(path),
            "cookieCount": 0,
            "cookieNames": [],
            "hasCsrf": False,
            "hasLgwCsrf": False,
            "error": "ixunfei-docx-writer cookie diagnostics are unavailable.",
        }
    path = cookie_path.expanduser()
    try:
        return cookie_diagnostics(path)
    except Exception as exc:
        return {
            "ok": False,
            "exists": path.exists(),
            "path": str(path),
            "cookieCount": 0,
            "cookieNames": [],
            "hasCsrf": False,
            "hasLgwCsrf": False,
            "error": f"{type(exc).__name__}: {exc}",
        }


def _engine_status(
    name: str,
    *,
    executable_lookup: ExecutableLookup,
    command_runner: CommandRunner,
) -> dict[str, object]:
    path = executable_lookup(name)
    if not path:
        return {"ok": False, "path": "", "version": ""}
    try:
        completed = command_runner(
            [name, "--version"],
            check=False,
            capture_output=True,
            text=True,
            errors="replace",
        )
    except Exception as exc:
        return {"ok": False, "path": path, "version": "", "error": str(exc)}
    raw_version = str(getattr(completed, "stdout", "") or getattr(completed, "stderr", "")).strip()
    version = raw_version.splitlines()[0] if raw_version else ""
    return {
        "ok": int(getattr(completed, "returncode", 1)) == 0,
        "path": path,
        "version": version,
    }


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
    executable_lookup: ExecutableLookup = shutil.which,
    command_runner: CommandRunner = subprocess.run,
) -> dict[str, object]:
    engines = {
        name: _engine_status(
            name,
            executable_lookup=executable_lookup,
            command_runner=command_runner,
        )
        for name in ("ixfdoc", "ixfwrite")
    }
    skills = _skills_status(home, env)
    cookies = _load_cookie_diagnostics(cookies_path)
    engines_ok = all(bool(status["ok"]) for status in engines.values())
    skills_ok = any(bool(status["ok"]) for status in skills.values())
    cookies_ok = bool(cookies.get("ok"))
    return {
        "ok": engines_ok and skills_ok and cookies_ok,
        "version": __version__,
        "engines": engines,
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
    engines = payload.get("engines", {})
    if isinstance(engines, Mapping):
        for name, status in engines.items():
            if not isinstance(status, Mapping):
                continue
            state = "ok" if status.get("ok") else "missing"
            detail = status.get("version") or status.get("path") or ""
            lines.append(f"engine {name} {state} {detail}".rstrip())
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
