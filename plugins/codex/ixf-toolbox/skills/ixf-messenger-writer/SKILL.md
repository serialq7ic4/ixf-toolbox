---
name: ixf-messenger-writer
description: Use when sending an approved i讯飞 Messenger message to a confirmed person or conversation through local Toolbox automation.
---

# ixf Messenger Writer

Use `ixf messenger send` through the local Toolbox CLI. This skill can send real messages only after dry-run planning and explicit user confirmation. Messenger automation is Chrome/Chromium-only and always runs against cloned profiles.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Runtime Resolution

Resolve the Go `ixf` runtime before running a business command.

1. Prefer `ixf` found on `PATH`.
2. Let `skillFile` be this installed `SKILL.md`. Resolve the plugin root with exactly `filepath.Dir(filepath.Dir(filepath.Dir(skillFile)))`; do not assume the current directory, a repository checkout, a fixed cache path, or a host-specific environment variable. Read `<plugin-root>/runtime.json`.
3. If `ixf` is absent from `PATH`, inspect the documented user-local path: `~/.local/share/ixf-toolbox/bin/ixf` on Unix or `%LOCALAPPDATA%\ixf-toolbox\bin\ixf.exe` on Windows.
4. Run the candidate's `ixf --version` and compare it with `runtime.json.minimumVersion`.
5. If the runtime is missing or too old, select `<plugin-root>/scripts/bootstrap-runtime.sh` or `<plugin-root>/scripts/bootstrap-runtime.ps1`.
6. Run the packaged bootstrap with `--dry-run` before `--apply`. Show the version, platform derived from the asset, release URL host, and target path.
7. Do not download or install anything without explicit user confirmation.
8. After confirmation, run bootstrap `--apply`, invoke the returned installed path directly, then run `ixf doctor --json`.
9. Run `ixf doctor --json` after bootstrap succeeds. Then continue the original request.

Use `ixf deps install`, not bootstrap, for Mermaid dependencies. Never invoke dependency installation silently.

## Workflow

1. Confirm the exact recipient or conversation, target mode, and message text.
2. Run readiness checks:
   `ixf messenger doctor --json`
3. Run dry-run send planning:
   `ixf messenger send --to "<target>" --mode person|conversation --message "<text>" --dry-run --json`
4. Show the dry-run plan and ask for explicit approval before applying.
5. If approved, send with:
   `ixf messenger send --to "<target>" --mode person|conversation --message "<text>" --apply --json`
6. Treat `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and `verifiedPresent:true` as the success condition. The command performs fresh-session verification before reporting success.

## Safety

Never send on ambiguous intent. Do not type into Messenger manually or fall back to another script. Do not use `open --apply` as a substitute for sending. Command output should not echo full message bodies; rely on message length and verification booleans.
