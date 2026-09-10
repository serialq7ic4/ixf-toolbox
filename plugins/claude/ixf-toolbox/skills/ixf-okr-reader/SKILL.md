---
name: ixf-okr-reader
description: Use when reading authorized i讯飞 OKR pages to summarize objectives, key results, mentions, ownership, alignment, or planning inputs.
---

# ixf OKR Reader

Use `ixf okr read` through the local Toolbox CLI. This skill is read-only.

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

1. Accept only OKR links the user is authorized to access.
2. Export cookies first if the local session is missing:
   `ixf cookies export --provider auto`
3. Read the OKR page:
   `ixf okr read "<okr-url>"`
4. Summarize Objective/KR content, mentions, and alignment points.
5. Keep any copied output local unless the user asks to persist it.

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, person IDs, OKR IDs, or sensitive personnel information unless necessary for the user's requested analysis.
