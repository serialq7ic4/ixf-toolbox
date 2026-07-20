---
name: ixf-docs-writer
description: Use when publishing local Markdown as a new authorized i讯飞 docx document after the user has approved the create target.
---

# ixf Docs Writer

Use `ixf docs publish` through the local Toolbox CLI. The command is an API-only, create-only native writer for a new docx document. It does not modify existing docx content, so decline requests to overwrite an original document until `ixf docs update` is available.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Confirm the Markdown file and destination URL or parent location for the new docx.
2. Run a dry run first:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --dry-run`
3. Review the planned title, create-only target, and required text checks with the user.
4. Apply only after explicit approval:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --apply`
5. Re-read or inspect the result when a verification URL is available.

## Safety

Do not invent document content. Do not write to ambiguous targets. Do not commit cookies, CSRF tokens, private URLs, document IDs, private response payloads, or generated private artifacts.
