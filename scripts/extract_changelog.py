from __future__ import annotations

import re
import sys
from pathlib import Path


def extract_version_section(markdown: str, version: str) -> str:
    pattern = re.compile(
        rf"^##\s+v?{re.escape(version)}[^\n]*\n(?P<body>.*?)(?=^##\s+|\Z)",
        re.M | re.S,
    )
    match = pattern.search(markdown)
    if match is None:
        raise ValueError(f"CHANGELOG section not found for version {version}")
    body = match.group("body").strip()
    if not body:
        raise ValueError(f"CHANGELOG section is empty for version {version}")
    return body + "\n"


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) != 2:
        print("usage: extract_changelog.py <version> <CHANGELOG.md>", file=sys.stderr)
        return 2
    version, changelog_path = args
    try:
        markdown = Path(changelog_path).read_text(encoding="utf-8")
        print(extract_version_section(markdown, version), end="")
    except Exception as exc:
        print(f"ERROR {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
