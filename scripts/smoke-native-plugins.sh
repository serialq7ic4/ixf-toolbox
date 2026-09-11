#!/bin/sh
set -eu

host_lifecycle=false
case "$#" in
    0) ;;
    1)
        if [ "$1" != "--host-lifecycle" ]; then
            echo "usage: scripts/smoke-native-plugins.sh [--host-lifecycle]" >&2
            exit 2
        fi
        host_lifecycle=true
        ;;
    *)
        echo "usage: scripts/smoke-native-plugins.sh [--host-lifecycle]" >&2
        exit 2
        ;;
esac

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
host_os=$(uname -s)
smoke_root=$(mktemp -d "${TMPDIR:-/tmp}/ixf-toolbox-plugin-smoke.XXXXXX")
trap 'rm -rf "$smoke_root"' EXIT HUP INT TERM

smoke_home="$smoke_root/home"
mkdir -p "$smoke_home/.codex" "$smoke_home/.claude" "$smoke_home/.config" "$smoke_home/localappdata"
printf '{}\n' > "$smoke_home/.claude/.claude.json"
powershell_localappdata="$smoke_home/localappdata"
if command -v cygpath >/dev/null 2>&1; then
    powershell_localappdata=$(cygpath -w "$powershell_localappdata")
fi

cd "$repo_root"
HOME="$smoke_home" XDG_CONFIG_HOME="$smoke_home/.config" go run ./cmd/pluginpack --check
HOME="$smoke_home" XDG_CONFIG_HOME="$smoke_home/.config" go test . -run '^TestNativePluginArtifacts$' -count=1

case "$host_os" in
    Darwin|Linux)
        HOME="$smoke_home" plugins/codex/ixf-toolbox/scripts/bootstrap-runtime.sh --dry-run >/dev/null
        HOME="$smoke_home" plugins/claude/ixf-toolbox/scripts/bootstrap-runtime.sh --dry-run >/dev/null
        ;;
esac

case "$host_os" in
    MINGW*|MSYS*|CYGWIN*)
        if command -v pwsh >/dev/null 2>&1; then
            HOME="$smoke_home" LOCALAPPDATA="$powershell_localappdata" \
                pwsh -NoProfile -File plugins/codex/ixf-toolbox/scripts/bootstrap-runtime.ps1 -DryRun >/dev/null
            HOME="$smoke_home" LOCALAPPDATA="$powershell_localappdata" \
                pwsh -NoProfile -File plugins/claude/ixf-toolbox/scripts/bootstrap-runtime.ps1 -DryRun >/dev/null
        fi
        ;;
esac

if command -v claude >/dev/null 2>&1; then
    HOME="$smoke_home" XDG_CONFIG_HOME="$smoke_home/.config" \
        claude plugin validate --strict plugins/claude/ixf-toolbox >/dev/null
fi

if [ "$host_lifecycle" != true ]; then
    exit 0
fi

command -v codex >/dev/null 2>&1 || {
    echo "codex is required for --host-lifecycle" >&2
    exit 1
}
command -v claude >/dev/null 2>&1 || {
    echo "claude is required for --host-lifecycle" >&2
    exit 1
}

HOME="$smoke_home" CODEX_HOME="$smoke_home/.codex" XDG_CONFIG_HOME="$smoke_home/.config" \
    codex plugin marketplace add "$repo_root" >/dev/null
HOME="$smoke_home" CODEX_HOME="$smoke_home/.codex" XDG_CONFIG_HOME="$smoke_home/.config" \
    codex plugin add ixf-toolbox@ixf-toolbox >/dev/null
codex_plugins=$(HOME="$smoke_home" CODEX_HOME="$smoke_home/.codex" XDG_CONFIG_HOME="$smoke_home/.config" \
    codex plugin list --json)
codex_compact=$(printf '%s' "$codex_plugins" | tr -d '[:space:]')
case "$codex_compact" in
    *'"name":"ixf-toolbox"'*'"installed":true'*'"enabled":true'*) ;;
    *)
        echo "Codex did not report installed and enabled ixf-toolbox" >&2
        exit 1
        ;;
esac
test ! -d plugins/codex/ixf-toolbox/hooks

HOME="$smoke_home" CLAUDE_CONFIG_DIR="$smoke_home/.claude" XDG_CONFIG_HOME="$smoke_home/.config" \
    claude plugin marketplace add "$repo_root" >/dev/null
HOME="$smoke_home" CLAUDE_CONFIG_DIR="$smoke_home/.claude" XDG_CONFIG_HOME="$smoke_home/.config" \
    claude plugin install ixf-toolbox@ixf-toolbox --scope user --yes >/dev/null
claude_plugins=$(HOME="$smoke_home" CLAUDE_CONFIG_DIR="$smoke_home/.claude" XDG_CONFIG_HOME="$smoke_home/.config" \
    claude plugin list --json)
claude_compact=$(printf '%s' "$claude_plugins" | tr -d '[:space:]')
case "$claude_compact" in
    *'"id":"ixf-toolbox@ixf-toolbox"'*'"enabled":true'*) ;;
    *)
        echo "Claude did not report installed and enabled ixf-toolbox" >&2
        exit 1
        ;;
esac

hook_output=$(printf '{}\n' | bash plugins/claude/ixf-toolbox/hooks/session-start)
case "$hook_output" in
    *'"hookEventName":"SessionStart"'*'i讯飞/LarkShell'*'ixf-toolbox:using-ixf-toolbox'*) ;;
    *)
        echo "Claude SessionStart hook did not emit the routing hint" >&2
        exit 1
        ;;
esac

prompt_hook_output=$(printf '%s\n' '{"prompt":"Read https://yf2ljykclb.xfchat.iflytek.com/docx/example123"}' | bash plugins/claude/ixf-toolbox/hooks/user-prompt-submit)
case "$prompt_hook_output" in
    *'"hookEventName":"UserPromptSubmit"'*'ixf-toolbox:using-ixf-toolbox'*'`ixf-toolbox` alone is not a valid identifier'*) ;;
    *)
        echo "Claude UserPromptSubmit hook did not emit the namespaced routing instruction" >&2
        exit 1
        ;;
esac

local_prompt_hook_output=$(printf '%s\n' '{"prompt":"Read ./README.md locally"}' | bash plugins/claude/ixf-toolbox/hooks/user-prompt-submit)
test -z "$local_prompt_hook_output"

bootstrap_target="$smoke_home/bootstrap"
case "$host_os" in
    Darwin|Linux)
        HOME="$smoke_home" plugins/codex/ixf-toolbox/scripts/bootstrap-runtime.sh --dry-run --install-dir "$bootstrap_target" >/dev/null
        test ! -e "$bootstrap_target/ixf"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        command -v pwsh >/dev/null 2>&1 || {
            echo "pwsh is required for Windows host lifecycle smoke" >&2
            exit 1
        }
        powershell_bootstrap_target="$bootstrap_target"
        if command -v cygpath >/dev/null 2>&1; then
            powershell_bootstrap_target=$(cygpath -w "$powershell_bootstrap_target")
        fi
        HOME="$smoke_home" LOCALAPPDATA="$powershell_localappdata" \
            pwsh -NoProfile -File plugins/codex/ixf-toolbox/scripts/bootstrap-runtime.ps1 -DryRun -InstallDir "$powershell_bootstrap_target" >/dev/null
        test ! -e "$bootstrap_target/ixf.exe"
        ;;
    *)
        echo "unsupported host lifecycle smoke platform: $host_os" >&2
        exit 1
        ;;
esac
