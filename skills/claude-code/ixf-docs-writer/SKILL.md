---
name: ixf-docs-writer
description: Use when publishing local Markdown into an authorized i讯飞 cloud document or modifying document content after the user has approved the write target.
---

# ixf Docs Writer

Use `ixf docs publish` through the local Toolbox CLI. The command is an API-only native writer. This skill can create or modify content, so confirm target, source, and write intent before applying.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Confirm the Markdown file and destination URL or parent location.
2. Run a dry run first:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --dry-run`
3. Review the planned title, target, and required text checks with the user.
4. Apply only after explicit approval:
   `ixf docs publish <file.md> --base-url https://tenant.example.test --apply`
5. Re-read or inspect the result when a verification URL is available.

## Safety

Do not invent document content. Do not write to ambiguous targets. Do not commit cookies, CSRF tokens, private URLs, document IDs, private response payloads, or generated private artifacts.
