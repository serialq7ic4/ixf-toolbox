from __future__ import annotations

import json
import subprocess
import sys
from collections.abc import Callable
from typing import Any


Runner = Callable[..., Any]


LEGACY_COMMAND_REPLACEMENTS = (
    ("ixfdoc cookies export", "ixf cookies export"),
    ("ixfdoc read", "ixf docs read"),
    ("ixfdoc outline", "ixf docs outline"),
    ("ixfdoc chunk", "ixf docs chunk"),
    ("ixfdoc cleanup", "ixf docs cleanup"),
    ("ixfdoc inspect", "ixf docs inspect"),
    ("ixfwrite docx publish", "ixf docs publish"),
    ("ixfwrite okr write", "ixf okr write"),
    ("ixfwrite cookies export", "ixf cookies export"),
    ("ixfwrite doctor", "ixf doctor"),
    ("ixfdoc ", "ixf docs "),
    ("ixfwrite ", "ixf "),
)


def rewrite_legacy_output(text: str | None) -> str:
    if not text:
        return ""
    rewritten = text
    for legacy, unified in LEGACY_COMMAND_REPLACEMENTS:
        rewritten = rewritten.replace(legacy, unified)
    return rewritten


def run_command(command: str, args: list[str], runner: Runner | None = None) -> int:
    selected_runner = runner or subprocess.run
    try:
        completed = selected_runner(
            [command, *args],
            check=False,
            capture_output=True,
            text=True,
            errors="replace",
        )
    except FileNotFoundError:
        payload = {
            "ok": False,
            "error": {
                "type": "dependency",
                "subtype": "engine_missing",
                "message": f"Required engine not found: {command}",
                "hint": "Install ixf-toolbox from the release wheel so bundled reader/writer dependencies are installed.",
                "retryable": False,
            },
        }
        print(json.dumps(payload, ensure_ascii=False), file=sys.stderr)
        return 127
    stdout = rewrite_legacy_output(getattr(completed, "stdout", ""))
    stderr = rewrite_legacy_output(getattr(completed, "stderr", ""))
    if stdout:
        sys.stdout.write(stdout)
    if stderr:
        sys.stderr.write(stderr)
    return int(getattr(completed, "returncode", 0))
