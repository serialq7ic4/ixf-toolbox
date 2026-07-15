from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ALLOWLIST = ROOT / "tests" / "python_runtime_imports_allowlist.txt"
IMPORT_RE = re.compile(r"^(from|import)\s+ixf_toolbox(?:\.|\s|$)")


def current_importers() -> list[str]:
    paths: list[str] = []
    for path in sorted((ROOT / "tests").glob("test_*.py")):
        for line in path.read_text(encoding="utf-8").splitlines():
            if IMPORT_RE.match(line):
                paths.append(path.relative_to(ROOT).as_posix())
                break
    return paths


def expected_importers() -> list[str]:
    return [
        line.strip()
        for line in ALLOWLIST.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.startswith("#")
    ]


def main() -> int:
    expected = expected_importers()
    actual = current_importers()
    if actual != expected:
        print("Python runtime import allowlist is stale", file=sys.stderr)
        print("Expected:", expected, file=sys.stderr)
        print("Actual:", actual, file=sys.stderr)
        return 1
    print("python runtime import allowlist is current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
