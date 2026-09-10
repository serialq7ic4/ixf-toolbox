---
name: using-ixf-toolbox
description: Use when a request mentions i讯飞, 讯飞文档, or LarkShell and involves /docx/, /wiki/, /base/, docx, wiki, sheets, bitable, OKR, or Messenger, including read, publish, update, patch, append-row, image upload, attachment upload, or message workflows; do not use for ordinary local Markdown reading or editing.
---

# Using ixf Toolbox

Use this as a lightweight routing skill for ixf Toolbox workflows. Users do not need to name this skill or any domain skill explicitly. Use background routing for natural user requests, then hand off to the correct domain skill or direct sheets CLI workflow.

High-signal intents include read, publish, update, patch, append row, upload image, and attachment workflows for i讯飞 resources.

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

## Routing

- Use `docs/agent-routing.md`, `AGENTS.md`, and current `skills/*/SKILL.md` files as authoritative current guidance.
- Do not route from historical implementation notes, old changelog entries, or `docs/superpowers/` plans.
- Use `ixf-docs-reader` for authorized document, wiki, docx, cloud-doc, embedded sheet, mindnote, image artifact, and direct sheets link reads through `ixf sheets read`.
- Ordinary local Markdown files do not require ixf Toolbox. For local `.md` inspection, summary, review, or edits, use the host filesystem. Use `ixf` for local Markdown only when the user explicitly needs chunking, artifact generation, publish, update, or patch workflows.
- Use `ixf-docs-writer` for approved Markdown publishing as a new docx document, localized insert under heading workflows, bounded one-section replace/delete workflows, or existing-docx update; existing-docx update can mean whole-body replacement through `ixf docs update`.
- For localized document insertion or append-under-heading requests, route to `ixf docs patch insert` through `ixf-docs-writer`; do not route these to `ixf docs update`.
- For confirmed one-section replacement or deletion requests, route to `ixf docs patch replace-section` or `ixf docs patch delete-section` through `ixf-docs-writer`; do not use those commands for simple insertion.
- For docx/wiki read or existing-docx write workflows, treat safe structure preflight as background metadata. `ixf docs read --out-dir` and write dry-runs expose `structure`; use `ixf docs structure --json` only when an explicit diagnostic or locator check is useful.
- For sheet cell update requests, use `ixf sheets update` directly: dry-run first, then `--apply` only after explicit approval and readback verification.
- For native docx table row append requests, use `ixf docs table append-row --dry-run --json`, mapping JSON fields to first-row headers and image cells to `{"file":"path"}`; after explicit approval use `--apply` and inspect `verify.ok`. Do not route native docs table edits through `ixf bitable` unless the target is a real base/bitable.
- For bitable record or attachment/image upload requests, use `ixf bitable inspect --url <url> --json`, `ixf bitable record create --dry-run --json`, `ixf bitable record create --apply --json`, `ixf bitable attach --dry-run --json`, or `ixf bitable attach --apply --json`; do not route those requests through docs or sheets commands. `ixf bitable record create --apply` supports confirmed API-only creation for text and attachment fields and appends to the current view by default; pass `--insert-position top` only when the user explicitly wants top insertion; `ixf bitable attach --apply` supports confirmed API-only uploads into an existing attachment field, preserves existing attachments, and verifies by readback.
- Use `ixf-okr-reader` for authorized OKR reading, summary, ownership, mention, or alignment analysis.
- Use `ixf-okr-writer` for approved Objective and Key Result creation or modification.
- Use `ixf-messenger-reader` for authorized i讯飞 Messenger readiness checks and read-only message inspection workflows.
- Use `ixf-messenger-writer` for approved Messenger sends after dry-run planning and explicit apply confirmation.

## Decision Rules

1. Classify the request as docs, sheets, bitable, OKR, or messenger.
2. Classify the intent as read or write.
3. Default ambiguous intent to read-only. Default to read-only when uncertain.
4. For writes, confirm the exact target and content, then follow the relevant writer skill or sheet CLI dry-run-first workflow.
5. For direct sheets link reads, prefer `ixf sheets read`; for embedded sheet reads inside docx, use `ixf docs read --expand-sheets`.
6. For localized docs insert requests, use `ixf docs patch insert --dry-run`, inspect `structure`, show `duplicateCandidate` and `existingBlocksTouched`, then use `--apply` only after approval.
7. For one-section replace/delete requests, use `ixf docs patch replace-section` or `ixf docs patch delete-section --dry-run`, inspect `structure`, show complex/outside-section safety metadata, then use `--apply` only after approval.
8. For sheets update requests, do not use `ixf docs update`; run `ixf sheets update --dry-run`, show the plan, then use `--apply` only after approval.
9. For native docs table row append requests, run `ixf docs table append-row --dry-run`, show `tableIndex`, `headers`, `plannedCellCount`, and `plannedImageCount`, then use `--apply` only after explicit approval and inspect `verify.ok`.
10. For bitable record create requests, run `ixf bitable record create --dry-run`, show `insertPosition`, `plannedRecordIndex`, and the field plan, then use `--apply` only after explicit approval and inspect `verify.ok` plus `verify.recordIndex`; for `ixf bitable attach`, run `--dry-run`, show the matched record and file plan, then use `--apply` only after explicit approval and inspect `verify.ok` plus `verify.recordId`.
11. If local authentication or installed routing looks unclear, run `ixf doctor --json` and inspect `agentRouting`.
12. If local authentication looks missing, run or suggest `ixf cookies export --provider auto`.

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, document IDs, OKR IDs, person IDs, or generated private artifacts unless the user explicitly needs that content for the requested analysis.
