#!/bin/sh
set -eu

python -m compileall -q src
python -m pytest -q

wheel_count="$(find dist -maxdepth 1 -type f -name 'ixf_toolbox-*.whl' | wc -l | tr -d '[:space:]')"
if [ "$wheel_count" -ne 1 ]; then
    echo "expected exactly one wheel under dist/, found $wheel_count" >&2
    exit 1
fi

wheel="$(find dist -maxdepth 1 -type f -name 'ixf_toolbox-*.whl' -print)"
smoke_root="$(mktemp -d "${TMPDIR:-/tmp}/ixf-toolbox-smoke.XXXXXX")"
trap 'rm -rf "$smoke_root"' EXIT HUP INT TERM

venv_dir="$smoke_root/venv"
smoke_home="$smoke_root/home"
mkdir -p "$smoke_home"
python -m venv "$venv_dir"

venv_python="$venv_dir/bin/python"
venv_ixf="$venv_dir/bin/ixf"
"$venv_python" -m pip install --force-reinstall "$wheel"

package_version=$("$venv_python" -c 'import importlib.metadata; print(importlib.metadata.version("ixf-toolbox"))')
cli_version=$(HOME="$smoke_home" "$venv_ixf" --version)
if [ "$cli_version" != "ixf $package_version" ]; then
    echo "CLI version mismatch: $cli_version != ixf $package_version" >&2
    exit 1
fi

printf '%s\n' "$cli_version"
HOME="$smoke_home" "$venv_ixf" --help >/dev/null
HOME="$smoke_home" "$venv_ixf" doctor --json >/dev/null || true
HOME="$smoke_home" "$venv_ixf" setup skills --runtimes codex --json >/dev/null

skill_path="$smoke_home/.codex/skills/ixf-docs-reader/SKILL.md"
test -f "$skill_path"
grep -q "ixf docs read" "$skill_path"

sample="$smoke_root/sample.md"
out_dir="$smoke_root/out"
printf '# Smoke\n\nToolbox smoke test.\n' > "$sample"
HOME="$smoke_home" "$venv_ixf" docs read "$sample" --out-dir "$out_dir" --print-manifest >/dev/null
test -f "$out_dir/manifest.json"
