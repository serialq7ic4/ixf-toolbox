---
name: ixf-docs-reader
description: Use when reading authorized i讯飞 cloud document, wiki, docx, mindnote, bitable, direct sheets link, embedded sheet, or local Markdown sources into local artifacts for analysis.
---

# ixf Docs Reader

Use `ixf docs read` through the local Toolbox CLI. This skill is read-only.
For a direct sheets link, prefer `ixf sheets read` so the request stays on the
dedicated sheet API surface.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Accept only sources the user is authorized to access.
2. Export cookies first if the local session is missing:
   `ixf cookies export --provider auto`
3. Read direct sheets links with:
   `ixf sheets read "<source>"`
4. Read other sources into a temporary output directory:
   `ixf docs read "<source>" --out-dir <dir> --print-manifest`
5. For embedded sheets inside a docx, use:
   `ixf docs read "<source>" --out-dir <dir> --expand-sheets --print-manifest`
6. Analyze generated Markdown/TSV artifacts.
7. Do not commit private artifacts unless the user explicitly asks.

## Commands

```bash
ixf docs inspect "<source>" --json
ixf sheets read "<direct-sheets-link>"
ixf docs read "<source>" --out-dir /tmp/ixf-docs --print-manifest
ixf docs read "<source>" --out-dir /tmp/ixf-docs --expand-sheets --print-manifest
ixf docs outline /tmp/ixf-docs/document.md --json
ixf docs chunk /tmp/ixf-docs/document.md --index 0
ixf docs cleanup /tmp/ixf-docs
```

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, document IDs, or generated private content unless needed for the user's requested analysis.
