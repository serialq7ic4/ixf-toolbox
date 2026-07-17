---
name: ixf-docs-reader
description: Use when reading authorized i讯飞 cloud document, wiki, docx, mindnote, bitable, direct sheets link, embedded sheet, or local Markdown sources into local artifacts for analysis.
---

# ixf Docs Reader

Use `ixf docs read` through the local Toolbox CLI. This skill is read-only.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Accept only sources the user is authorized to access.
2. Export cookies first if the local session is missing:
   `ixf cookies export --provider auto`
3. Read sources into a temporary output directory:
   `ixf docs read "<source>" --out-dir <dir> --print-manifest`
4. Analyze generated Markdown/TSV artifacts.
5. Do not commit private artifacts unless the user explicitly asks.

## Commands

```bash
ixf docs inspect "<source>" --json
ixf docs read "<source>" --out-dir /tmp/ixf-docs --print-manifest
ixf docs outline /tmp/ixf-docs/document.md --json
ixf docs chunk /tmp/ixf-docs/document.md --index 0
ixf docs cleanup /tmp/ixf-docs
```

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, document IDs, or generated private content unless needed for the user's requested analysis.
