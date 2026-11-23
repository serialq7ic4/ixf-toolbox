---
name: ixf-servicedesk-writer
description: Use when the user explicitly asks to create, submit, report, repair through, or withdraw a ServiceDesk ticket, including requests that rely on facts already established in the current conversation.
---

# ixf ServiceDesk Writer

Use `ixf servicedesk` through the local Toolbox CLI. Creating and withdrawing are real remote mutations. Every write requires a current dry-run, an exact target/content summary, and explicit confirmation immediately before `--apply`.

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

## Intent Gate

Use this skill only when the user expresses a ticket action such as creating/submitting a ticket, reporting a repair through ServiceDesk, or withdrawing a ticket. A fault, server, disk, or business problem by itself does not authorize a ticket workflow.

## Create Workflow

1. Run `ixf servicedesk profile list --json` and inspect plausible profiles with `ixf servicedesk profile show <name> --json`.
2. Select the profile with Agent semantic judgment over its description, intended use, exclusions, and examples. Do not use keyword scoring. If multiple profiles are reasonable, present them and ask the user to choose.
3. Run `ixf servicedesk template inspect --profile <name> --json` to obtain the current live input contract.
4. Reuse facts already established in the conversation, including server, device SN, component, symptoms, and requested time. Ask only for required facts that remain missing. Never infer or invent a value.
5. Write a strict JSON input file containing only public contract field names. Keep it outside the repository when it contains private operational data.
6. Run:
   `ixf servicedesk create --profile <name> --input <file> --dry-run --json`
7. Stop on field issues, destructive drift, stale Registry, unsupported conditions, unsupported service key, or `apply.blocked:true`.
8. Show the user the service name, title and business facts, provided/defaulted field counts, table row count, drift status, and whether apply is blocked. Ask for explicit confirmation for this exact ticket.
9. After confirmation, use the same profile and input file with:
   `ixf servicedesk create --profile <name> --input <file> --apply --json`
10. Claim success only when the result reports `applied:true`, a ticket ID/detail URL, and readable-state verification. For `outcome:"unknown"` or a readback failure, do not retry; report the safe ticket reference and inspect it first.

## Withdraw Workflow

1. If the user supplies a detail URL or explicit business/internal ID pair, use it as the candidate. Otherwise identify candidates from the current conversation with:
   `ixf servicedesk search --mine --role any --query <text> --json`
   For a broad request about the user's unfinished submissions, use:
   `ixf servicedesk list --mine --role submitted --state open --json`
2. Match the user's business description against ticket titles, services, state, update time, and current conversation context using Agent semantic judgment. Search and list are read-only candidate discovery, not authorization to withdraw. If zero or multiple plausible tickets remain, ask the user instead of guessing. When `complete:false`, disclose `failedScopes` and do not infer that the candidate set is exhaustive or unique. Stop on `query_failed`; it is not an empty candidate list.
3. Use the candidate's actual `businessId` and internal `id`, not its `REQ...` display number or the bootstrap business ID. Inspect the exact candidate before planning the write:
   `ixf servicedesk get --business-id <ticket.businessId> --ticket-id <ticket.id> --json`
4. Run a dry-run against the same exact target:
   `ixf servicedesk withdraw --business-id <ticket.businessId> --ticket-id <ticket.id> --reason <reason> --dry-run --json`
5. Show ticket number, title, service, current state, and reason. Obtain new explicit confirmation specifically for withdrawing this ticket.
6. Apply with the same business ID, internal ID, and reason using `--apply --json`.
7. Claim success only when `applied:true` and `verify.stateWithdrawn:true` after readback.

## Authentication And Safety

Never ask the user to paste an API Key into chat or put it in a CLI argument. Never print private input payloads unnecessarily, store credentials in a profile, retry an unknown create outcome, or use a revoke/terminate endpoint as a substitute for withdrawal. Design approval, read-only candidate selection, a prior create confirmation, or a general “continue” is not fresh confirmation for a later create or withdrawal mutation.
