---
name: ixf-docs-writer
description: Use when publishing local Markdown as a new i讯飞 docx document or preflighting an approved existing-docx update.
---

# ixf Docs Writer

Use `ixf docs publish` through the local Toolbox CLI for new documents. The command is API-only and create-only for a new docx document. It does not modify existing docx content.

Use `ixf docs update` for existing docx update preflight only. `--apply` is not supported in this version, so do not claim that original docx content was changed.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. For new docx publishing, confirm the Markdown file and destination URL or parent location.
2. Run a publish dry run first:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --dry-run`
3. Review the planned title, create-only target, and required text checks with the user.
4. Apply only after explicit approval:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --apply`
5. For existing docx update requests, run preflight only:
   `ixf docs update <file.md> --url https://tenant.example.test/docx/example --dry-run`
6. Re-read or inspect the result when a verification URL is available.

## Safety

Do not invent document content. Do not write to ambiguous targets. Do not commit cookies, CSRF tokens, private URLs, document IDs, private response payloads, or generated private artifacts.
