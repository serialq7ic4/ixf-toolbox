from __future__ import annotations

import json
import subprocess
import sys
from collections.abc import Callable
from typing import Any


Runner = Callable[..., Any]


def run_command(command: str, args: list[str], runner: Runner | None = None) -> int:
    selected_runner = runner or subprocess.run
    try:
        completed = selected_runner([command, *args], check=False)
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
    return int(getattr(completed, "returncode", 0))
