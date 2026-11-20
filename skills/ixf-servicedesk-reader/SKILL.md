---
name: ixf-servicedesk-reader
description: Use when listing supported ServiceDesk ticket types, inspecting a live ticket form, validating a shared profile, listing the current user's recent or unfinished tickets, or checking a ticket's current status without creating or withdrawing it.
---

# ixf ServiceDesk Reader

Use `ixf servicedesk` through the local Toolbox CLI. This skill is read-only. It can read the shared semantic Registry and the live ServiceDesk API, but it never creates or withdraws a ticket. ServiceDesk writes require the writer skill and explicit confirmation.

## Runtime Boundary

Go `ixf` only. Do not use Python fallback, UI automation, browser network capture, a central proxy, or arbitrary HTTP replay. Do not call `ixfdoc` or `ixfwrite`.

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

1. To answer which ticket types are currently supported, run:
   `ixf servicedesk profile list --json`
2. Present profile names, descriptions, and intended use in business language. Do not expose backend field keys or ask the CLI to score keywords.
3. When the user selects or describes a type, use Agent semantic judgment over each profile's description, routing guidance, examples, and exclusions. If more than one profile remains plausible, ask the user to choose.
4. Read one selected profile with:
   `ixf servicedesk profile show <name> --json`
5. Inspect its current live input contract with:
   `ixf servicedesk template inspect --profile <name> --json`
6. To list the current user's tickets, obtain an explicit business ID from configured doctor output or the selected profile, then run:
   `ixf servicedesk list --mine --business-id <id> --view recent|open --json`
7. Report `complete:false` and `failedScopes` as partial results. An unknown state remains visible in the open view and must be described as unknown.
8. Query one known ticket with:
   `ixf servicedesk get --url <detail-url> --json`

Use `recent` for “最近提了哪些工单” and `open` for “哪些工单还没完成”. Include the ticket number, title, service, state, current step or processors, creation time, and detail URL when available.

## Authentication

Run `ixf servicedesk doctor --json` when readiness is unclear. Never ask the user to paste an API Key into chat or pass it as a command-line value. The CLI reads `IXF_SERVICEDESK_API_KEY`, `--api-key-file`, or the private default credential file and never prints the key or its length.

## Safety

Do not infer a ticket action from an ordinary fault description. “服务器硬盘坏了，帮我分析原因” is analysis, not a request to open a ticket. Do not invent missing business facts, treat a partial list as complete, or claim an unknown server state is finished.
