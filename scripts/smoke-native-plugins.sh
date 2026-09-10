#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
smoke_root=$(mktemp -d "${TMPDIR:-/tmp}/ixf-toolbox-plugin-smoke.XXXXXX")
trap 'rm -rf "$smoke_root"' EXIT HUP INT TERM

smoke_home="$smoke_root/home"
mkdir -p "$smoke_home/.config" "$smoke_home/localappdata"

cd "$repo_root"
HOME="$smoke_home" XDG_CONFIG_HOME="$smoke_home/.config" go run ./cmd/pluginpack --check
HOME="$smoke_home" XDG_CONFIG_HOME="$smoke_home/.config" go test . -run '^TestNativePluginArtifacts$' -count=1

case "$(uname -s)" in
    Darwin|Linux)
        HOME="$smoke_home" plugins/codex/ixf-toolbox/scripts/bootstrap-runtime.sh --dry-run >/dev/null
        HOME="$smoke_home" plugins/claude/ixf-toolbox/scripts/bootstrap-runtime.sh --dry-run >/dev/null
        ;;
esac

if command -v pwsh >/dev/null 2>&1; then
    HOME="$smoke_home" LOCALAPPDATA="$smoke_home/localappdata" \
        pwsh -NoProfile -File plugins/codex/ixf-toolbox/scripts/bootstrap-runtime.ps1 -DryRun >/dev/null
    HOME="$smoke_home" LOCALAPPDATA="$smoke_home/localappdata" \
        pwsh -NoProfile -File plugins/claude/ixf-toolbox/scripts/bootstrap-runtime.ps1 -DryRun >/dev/null
fi

if command -v claude >/dev/null 2>&1; then
    HOME="$smoke_home" XDG_CONFIG_HOME="$smoke_home/.config" \
        claude plugin validate --strict plugins/claude/ixf-toolbox >/dev/null
fi
