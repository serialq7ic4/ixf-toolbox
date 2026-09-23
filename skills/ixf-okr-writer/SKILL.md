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

1. Inspect the page first with `ixf okr inspect "<okr-url>"` and show the user the current objectives, their indexes, and their KR counts. `--objective-index` is positional, so never target an index without seeing what is there. Use `inspect` rather than `read` for this: it reports indexes, identifiers, and counts as JSON, while `read` renders Markdown intended for a human.
   `nextObjectiveIndex` in that output is the index that would create a new objective; any index below it replaces an existing objective's KRs.
2. Confirm the OKR URL, objective index, and exact Objective/KR content.
3. Prepare JSON input locally with only the approved content.
   Shape: `{"objectives":[{"objective":"...","krs":["KR1","KR2","KR3"]}]}`
   The top level is an object with an `objectives` key, not an array. The key is `krs`; any other spelling is rejected. `krs` may not be empty.
4. Run dry run first:
   `ixf okr write --url "<okr-url>" --input okr.json --objective-index 3 --dry-run`
5. Check `krCount` in the dry-run output against the number of KRs intended, and state it to the user. `krCount` reports the input file, not the target, so the dry run cannot show how many existing KRs would be replaced — that is why step 1 inspects the page.
6. State plainly to the user that writing an existing objective replaces its whole KR set, and name how many KRs the target currently has, taken from the `krCount` of that objective in step 1. Get approval for that replacement specifically, not merely for the new content.
7. Apply only after explicit approval:
   `ixf okr write --url "<okr-url>" --input okr.json --objective-index 3 --apply`
8. Inspect `verify.ok` and `verify.comparedAgainst:"spec"`, and read `verify.scope`. The check confirms the stored KRs match what was sent, in order; it does not confirm other objectives were untouched.
9. Re-read the OKR page after writing and verify only the intended objective changed.

## Replacement Semantics

Writing an existing objective with `--objective-index N` **replaces its entire KR set**: the KRs in the input become the objective's KRs and the previous ones are removed. It is not an append. To add a KR while keeping the existing ones, include the existing KR texts in the input alongside the new one.

`--objective-index N` where N is exactly one past the current objective count creates a new objective instead of replacing one, so the same flag means different things depending on how many objectives exist. Confirm the current count from step 1 before choosing N.

Without `--objective-index`, objectives are matched by text and existing KRs are preserved; that path only removes KRs when `--prune` is passed.

An empty or absent `krs` is rejected, because it would delete the existing KRs and add nothing. There is no flag that means "clear this objective"; removing KRs without replacing them is not supported through this command.

## Safety

Do not modify O/KR content from vague instructions. Do not delete or prune unless explicitly requested. Do not commit OKR JSON files, cookies, CSRF tokens, private URLs, person IDs, OKR IDs, or private API payloads.
