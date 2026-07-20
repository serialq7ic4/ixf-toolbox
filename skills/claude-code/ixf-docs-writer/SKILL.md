---
name: ixf-docs-writer
description: Use when publishing local Markdown as a new i讯飞 docx document or updating an approved existing docx document.
---

# ixf Docs Writer

Use `ixf docs publish` through the local Toolbox CLI for new documents. The command is API-only and create-only for a new docx document. It does not modify existing docx content.

Use `ixf docs update` for existing docx updates. The supported mode is `replace_body`: it keeps the original URL, permissions, and location, but replaces the body blocks. It rejects complex existing content by default; use `--allow-complex-replace` only after explicit destructive approval.

This skill does not edit embedded or direct sheet cell data. For sheet cell
update requests, use `ixf sheets update --dry-run` only; `ixf sheets update
--apply` is unavailable until a real sheet write API contract is captured.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. For new docx publishing, confirm the Markdown file and destination URL or parent location.
2. Run a publish dry run first:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --dry-run`
3. Review the planned title, create-only target, and required text checks with the user.
4. Apply only after explicit approval:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --apply`
5. For existing docx update requests, run update dry-run first:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --dry-run`
6. Apply existing docx updates only after explicit approval:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --apply`
7. If dry-run reports complex blocks, do not apply unless the user explicitly approves losing those blocks:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --allow-complex-replace --apply`
8. If dry-run reports `tableFallbackCount>0`, tell the user Markdown tables will be preserved as readable callout fallback blocks, not native table/sheet blocks.
9. After apply, inspect `verify.ok`, `verify.missingRequiredText`, and `verify.emptyCalloutCount`; do not claim success if required text is missing or empty callouts are reported.
10. For sheets update requests, do not use `ixf docs update`; show only an `ixf sheets update --dry-run` plan.
11. Re-read or inspect the result when a verification URL is available.

## Safety

Do not invent document content. Do not write to ambiguous targets. Do not commit cookies, CSRF tokens, private URLs, document IDs, private response payloads, or generated private artifacts.
