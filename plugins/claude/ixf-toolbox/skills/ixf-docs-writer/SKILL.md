---
name: ixf-docs-writer
description: Use when publishing local Markdown as a new i讯飞 docx document or updating an approved existing docx document.
---

# ixf Docs Writer

Use `ixf docs publish` through the local Toolbox CLI for new documents. The command is API-only and create-only for a new docx document. It does not modify existing docx content.

Use `ixf docs patch insert` for localized insertion under an existing heading, such as "insert this table under heading 1.1". Treat natural "insert under heading" requests as patch insert requests. This mode adds new blocks without replacing the document body. For localized insertion, do not use `ixf docs update` for localized insertion.

Use `ixf docs patch replace-section` or `ixf docs patch delete-section` only for confirmed one-section replacement or deletion. These are bounded destructive operations; do not use them for simple insertion. They reject complex section content by default and require `--allow-complex-section-replace` only after explicit destructive approval.

Use `ixf docs update` for whole-body existing docx updates. The supported mode is `replace_body`: it keeps the original URL, permissions, and location, but replaces the body blocks. It rejects complex existing content by default; use `--allow-complex-replace` only after explicit destructive approval.

Existing-docx write dry-runs include a safe `structure` object with heading paths, section ranges, duplicate headings, and complex-block risk. Inspect it before choosing or applying a target; surface it to the user only when it affects ambiguity or write safety.

This skill does not edit embedded or direct sheet cell data. For sheet cell
update requests, do not use `ixf docs update`; route to `ixf sheets update`
with dry-run first and `--apply` only after explicit approval.

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

## Publish Readiness

When a user asks to publish or整理到 i讯飞文档, produce a publishable Markdown file and continue to `ixf docs publish --dry-run` whenever a tenant/base URL can be inferred or a default is configured. When possible, derive the tenant base URL from the user's i讯飞 link or explicitly provided destination. If no prompt URL exists, run `ixf doctor --json` and inspect `.docs.defaultBaseURL.configured`; when true, run `ixf docs publish <file.md> --dry-run` without `--base-url` and review `baseURLSource` plus `targetHost`. If no base URL, default, or parent location is available, ask only for the destination; do not stop at a local-only draft.

Do not treat top-level `doctor.ok=false` alone as an authentication failure. Inspect `.cookies.ok` and `.capabilities.docsPublish` from `ixf doctor --json`; if cookies are missing, run or ask for `ixf cookies export --provider auto`, then retry the dry-run.

Markdown Mermaid blocks publish/update/patch as image blocks, not code blocks. This includes `` ```mermaid `` fenced code and exported `` ```Plain `` blocks whose first non-empty content line starts with Mermaid diagram keywords such as `flowchart`, `sequenceDiagram`, or `erDiagram`. Dry-runs expose `mermaidImageCount`, `plannedImageCount`, `mermaidRendererAvailable`, `mermaidRendererReady`, `mermaidRendererError`, `mermaidRendererRemediation`, `mermaidPreferredFormat`, and `mermaidFallbackFormat`. Apply requires external Mermaid CLI `mmdc` on `PATH` and a healthy render probe; SVG is preferred and PNG is the fallback. If `mermaidImageCount>0` and `mermaidRendererReady=false`, do not apply. Run `ixf deps install --dry-run --json`, show the plan, and use `ixf deps install --apply --json` only after explicit approval; alternatively set `PUPPETEER_EXECUTABLE_PATH` to an installed Chrome/Chromium binary.

## Document Size

One write is one request, and the endpoint accepts at most **3500 `change_map` entries** per write: one page root entry plus 3499 block entries. Over that the server returns `code=4000002 invalid param` and names no ceiling.

The limit counts entries, not bytes or lines. 200 blocks carrying 3.0 MB published fine while 3860 blocks carrying 1.4 MB failed. Tables expand into row, cell, and text blocks, so a table-heavy 1322-line document reaches 4620 entries while a text-only 1642-line document sits near 820. Never estimate from line count, file size, or `plannedPayloadBytes`; the only predictor is `plannedChangeEntries` from a dry-run.

Every write dry-run reports `plannedBlockEntries`, `plannedChangeEntries`, and `maxChangeEntriesPerWrite`. Compare `plannedChangeEntries` against the limit, not `plannedBlockEntries`, which is one lower because it excludes the page root entry.

`ixf docs publish` handles oversized documents itself: when the content needs more than one write, it splits at top-level block boundaries and writes the parts in order. Dry-run reports `plannedWriteCount`, `willSplitWrites`, and `splitReason`; apply reports `writeCount` and `splitWrites`. No flag or manual skeleton is required. State the planned write count to the user before applying, because a split publish is several writes and can stop part-way.

If a split publish fails mid-sequence the document exists and is incomplete. The error names its URL and how many writes completed. Do not re-run the same command, which creates a second document: report the URL and completed write count, then add the remaining sections with `ixf docs patch insert --under-heading` after approval. This tool cannot delete a document, so removing an unwanted partial document is manual in the web UI.

Two cases publish cannot split:

- A single block larger than one write, such as a table of several thousand rows. Publish fails before writing anything and names the block; shorten it.
- `ixf docs update`, which replaces the whole body and is never split. A partial replace would leave the document truncated, so an oversized update fails loudly. Prefer keeping the document and adding material with `ixf docs patch insert`, one section per write.

`ixf docs patch insert` and `patch replace-section` are subject to the same per-write limit and are not split automatically. Their dry-runs report `plannedChangeEntries` and `fitsInOneWrite`; when `fitsInOneWrite` is false, split the fragment into smaller ones. `ixf docs outline` and `ixf docs chunk` split by **character** budget, which does not bound entry count, so always check `plannedChangeEntries` on each fragment's dry-run before applying.

## Workflow

1. For new docx publishing, confirm the Markdown file and destination URL, default publish base URL, or parent location.
2. Run a publish dry run first:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --dry-run`
   or, when `.docs.defaultBaseURL.configured=true`:
   `ixf docs publish <file.md> --dry-run`
3. Review the planned title, create-only target, Mermaid image metadata, and required text checks with the user. When `willSplitWrites` is true, also state `plannedWriteCount` and that a split publish can stop part-way, per Document Size.
4. Apply only after explicit approval:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --apply`
5. For localized insertion under a heading, create a fragment Markdown file and run patch dry-run first:
   `ixf docs patch insert <fragment.md> --url https://tenant.example.test/docx/example --under-heading "Target Heading" --dry-run`
6. Review `structure`, `duplicateCandidate:false`, `existingBlocksTouched:false`, `tableBlockType:"table"`, `tableFallbackCount:0`, and Mermaid image metadata. If `duplicateCandidate:true`, stop instead of applying.
7. Apply localized insertion only after explicit approval:
   `ixf docs patch insert <fragment.md> --url https://tenant.example.test/docx/example --under-heading "Target Heading" --require "critical content" --apply`
8. After patch apply, inspect `verify.ok`, `verify.unchangedExistingBlocks`, and `verify.missingRequiredText`; do not claim success unless `verify.ok=true` and `verify.unchangedExistingBlocks=true`.
9. For confirmed one-section replacement, run section replace dry-run first:
   `ixf docs patch replace-section <fragment.md> --url https://tenant.example.test/docx/example --under-heading "Target Heading" --dry-run`
10. For confirmed one-section deletion, run section delete dry-run first:
   `ixf docs patch delete-section --url https://tenant.example.test/docx/example --under-heading "Target Heading" --dry-run`
11. Review `structure`, `destructive:true`, `complexBlockTypes`, `outsideSectionBlocksTouched:false`, and the deleted/planned top-level block counts. If complex blocks are present, apply only with explicit approval and `--allow-complex-section-replace`.
12. After section replace/delete apply, inspect `verify.ok`, `verify.unchangedOutsideSectionBlocks`, and `verify.missingRequiredText`; do not claim success unless outside-section verification passes.
13. For existing docx whole-body update requests, run update dry-run first and inspect `structure` before applying:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --dry-run`
14. Apply existing docx whole-body updates only after explicit approval:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --apply`
15. If dry-run reports complex blocks, do not apply unless the user explicitly approves losing those blocks:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --allow-complex-replace --apply`
16. Markdown tables are expected to publish or patch as native docx table blocks; if `tableFallbackCount>0`, stop and investigate before applying.
17. After update apply, inspect `verify.ok`, `verify.missingRequiredText`, and `verify.emptyCalloutCount`; do not claim success if required text is missing or empty callouts are reported.
18. For sheets update requests, do not use `ixf docs update`; route to `ixf sheets update --dry-run`, then apply only after explicit approval.
19. Re-read or inspect the result when a verification URL is available.

## Safety

Do not invent document content. Do not write to ambiguous targets. Do not commit cookies, CSRF tokens, private URLs, document IDs, private response payloads, or generated private artifacts.
