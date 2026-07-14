#!/bin/sh
set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    echo "usage: scripts/smoke-go-binary.sh <ixf-binary> [expected-version]" >&2
    exit 2
fi

binary="$1"
expected_version="${2:-}"

if [ ! -f "$binary" ]; then
    echo "binary not found: $binary" >&2
    exit 1
fi

if [ ! -x "$binary" ]; then
    echo "binary is not executable: $binary" >&2
    exit 1
fi

smoke_root="$(mktemp -d "${TMPDIR:-/tmp}/ixf-toolbox-go-smoke.XXXXXX")"
trap 'rm -rf "$smoke_root"' EXIT HUP INT TERM

smoke_home="$smoke_root/home"
mkdir -p "$smoke_home"

version_output=$(HOME="$smoke_home" "$binary" --version)
if [ -n "$expected_version" ] && [ "$version_output" != "ixf $expected_version" ]; then
    echo "CLI version mismatch: $version_output != ixf $expected_version" >&2
    exit 1
fi

printf '%s\n' "$version_output"
HOME="$smoke_home" "$binary" --help >/dev/null
HOME="$smoke_home" "$binary" doctor --json >/dev/null || true
HOME="$smoke_home" "$binary" setup skills --runtimes codex --json >/dev/null

skill_path="$smoke_home/.codex/skills/ixf-docs-reader/SKILL.md"
test -f "$skill_path"
grep -q "ixf docs read" "$skill_path"

sample="$smoke_root/sample.md"
out_dir="$smoke_root/out"
printf '# Smoke\n\nToolbox Go binary smoke test.\n' > "$sample"
HOME="$smoke_home" "$binary" docs read "$sample" --out-dir "$out_dir" --print-manifest >/dev/null
test -f "$out_dir/manifest.json"
