---
name: ixf-messenger-reader
description: Use when inspecting authorized i讯飞 Messenger readiness, unread or recent messages, chat routing, or local browser-profile prerequisites without sending messages.
---

# ixf Messenger Reader

Use `ixf messenger` through the local Toolbox CLI. This skill is read-only and never sends messages. Messenger automation is Chrome/Chromium-only and always runs against a cloned profile, never the live LarkShell profile.

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

1. Confirm the user is asking for Messenger inspection or read-only message analysis.
2. Check local readiness:
   `ixf messenger doctor --json`
3. If the user wants unread or recent message content, start with a read plan:
   `ixf messenger read --scope unread|recent --dry-run --json`
4. If the user explicitly accepts that reading may mark opened chats as read, read through the cloned profile:
   `ixf messenger read --scope unread|recent --apply --json`
5. If the user wants to open a target, plan it without sending:
   `ixf messenger open --to "<target>" --mode person|conversation --dry-run --json`
6. If the user explicitly accepts that opening a chat may mark it as read, verify the target with:
   `ixf messenger open --to "<target>" --mode person|conversation --apply --json`
7. Treat `targetVerified:true` as the success condition for open verification.

## Safety

Do not print cookie values, CSRF tokens, private conversation IDs, screenshots, profile contents, or raw browser state. Message bodies may appear only when the user requested message reading or analysis. `read --apply` and `open --apply` never send messages and must not type into the editor.
