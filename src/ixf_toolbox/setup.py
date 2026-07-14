from __future__ import annotations

from contextlib import ExitStack
from dataclasses import dataclass
from importlib import resources
from pathlib import Path
import shutil
from typing import Iterable, Mapping


SKILL_NAMES = (
    "using-ixf-toolbox",
    "ixf-docs-reader",
    "ixf-docs-writer",
    "ixf-okr-reader",
    "ixf-okr-writer",
)


@dataclass(frozen=True)
class RuntimeTarget:
    key: str
    skills_dir: Path
    source_root: Path


def packaged_project_root():
    packaged = resources.files("ixf_toolbox").joinpath("_resources")
    if packaged.is_dir():
        return packaged
    source_root = Path(__file__).resolve().parents[2]
    if (source_root / "skills").is_dir():
        return source_root
    return packaged


def detect_runtime_targets(home: Path, env: Mapping[str, str]) -> list[RuntimeTarget]:
    codex_dir = Path(
        env.get("IXF_TOOLBOX_CODEX_SKILLS_DIR", home / ".codex" / "skills")
    ).expanduser()
    claude_dir = Path(
        env.get("IXF_TOOLBOX_CLAUDE_CODE_SKILLS_DIR", home / ".claude" / "skills")
    ).expanduser()
    return [
        RuntimeTarget("codex", codex_dir, Path("skills/codex")),
        RuntimeTarget("claude-code", claude_dir, Path("skills/claude-code")),
    ]


def normalize_runtimes(raw: Iterable[str]) -> list[str]:
    values = [item.strip().lower() for item in raw if item.strip()]
    if not values or "auto" in values or "all" in values:
        return ["codex", "claude-code"]
    if "none" in values:
        return []
    result: list[str] = []
    for value in values:
        normalized = {
            "claude": "claude-code",
            "claude_code": "claude-code",
        }.get(value, value)
        if normalized not in {"codex", "claude-code"}:
            raise ValueError(f"unsupported runtime: {value}")
        if normalized not in result:
            result.append(normalized)
    return result


def install_skill_wrappers(
    project_root,
    home: Path,
    runtimes: list[str],
    force: bool,
    env: Mapping[str, str],
) -> dict[str, object]:
    selected = set(normalize_runtimes(runtimes))
    installed: list[dict[str, str]] = []
    skipped: list[dict[str, str]] = []
    with ExitStack() as stack:
        for target in detect_runtime_targets(home, env):
            if target.key not in selected:
                continue
            for skill_name in SKILL_NAMES:
                source = stack.enter_context(
                    resources.as_file(project_root / target.source_root / skill_name)
                )
                destination = target.skills_dir / skill_name
                if destination.exists() and not force:
                    skipped.append(
                        {
                            "runtime": target.key,
                            "skill": skill_name,
                            "path": str(destination),
                            "reason": "exists",
                        }
                    )
                    continue
                if destination.exists():
                    shutil.rmtree(destination)
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copytree(source, destination)
                installed.append(
                    {
                        "runtime": target.key,
                        "skill": skill_name,
                        "path": str(destination),
                    }
                )
    return {"ok": True, "installed": installed, "skipped": skipped}
