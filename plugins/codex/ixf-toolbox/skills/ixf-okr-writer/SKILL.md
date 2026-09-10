---
name: ixf-okr-writer
description: Use when writing approved Objective and Key Result content into an authorized i讯飞 OKR page after the user has confirmed the exact target.
---

# ixf OKR Writer

Use `ixf okr write` through the local Toolbox CLI. The command is an API-only native writer. This skill can modify published OKR content, so use dry-run-first operation and apply only after explicit approval.

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

1. Confirm the OKR URL, objective index, and exact Objective/KR content.
2. Prepare JSON input locally with only the approved content.
   Shape: `{"objectives":[{"objective":"...","krs":["KR1","KR2","KR3"]}]}`
3. Run dry run first:
   `ixf okr write --url "<okr-url>" --input okr.json --objective-index 3 --dry-run`
4. Review the planned Objective/KR changes with the user.
5. Apply only after explicit approval:
   `ixf okr write --url "<okr-url>" --input okr.json --objective-index 3 --apply`
6. Re-read the OKR page after writing and verify only the intended objective changed.

## Safety

Do not modify O/KR content from vague instructions. Do not delete or prune unless explicitly requested. Do not commit OKR JSON files, cookies, CSRF tokens, private URLs, person IDs, OKR IDs, or private API payloads.
