---
name: ixf-docs-reader
description: Use when reading authorized i讯飞 cloud document, wiki, docx, mindnote, bitable, direct sheets link, embedded sheet, or generated image artifacts for analysis.
---

# ixf Docs Reader

Use `ixf docs read` through the local Toolbox CLI. This skill is read-only.
Remote docx/wiki reads include safe structure preflight metadata in the manifest
and write a `.structure.json` artifact when `--out-dir` is used.
For a direct sheets link, prefer `ixf sheets read` so the request stays on the
dedicated sheet API surface.

Ordinary local Markdown files do not require this skill. Use the host
filesystem for local `.md` inspection, summary, review, or edits. Use local
Markdown with `ixf` only when the user explicitly requests chunking, manifest
artifact generation, or a docs writer workflow that consumes Markdown.

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

1. Accept only sources the user is authorized to access.
2. Export cookies first if the local session is missing:
   `ixf cookies export --provider auto`
3. Read direct sheets links with:
   `ixf sheets read "<source>"`
4. Read other sources into a temporary output directory and inspect the manifest structure summary when it affects the analysis:
   `ixf docs read "<source>" --out-dir <dir> --print-manifest`
5. For embedded sheets inside a docx, use:
   `ixf docs read "<source>" --out-dir <dir> --expand-sheets --print-manifest`
6. Analyze generated Markdown/TSV artifacts.
7. Do not commit private artifacts unless the user explicitly asks.

## Commands

```bash
ixf docs inspect "<source>" --json
ixf docs structure "<doc-or-wiki-url>" --json
ixf sheets read "<direct-sheets-link>"
ixf docs read "<source>" --out-dir /tmp/ixf-docs --print-manifest
ixf docs read "<source>" --out-dir /tmp/ixf-docs --expand-sheets --print-manifest
ixf docs outline /tmp/ixf-docs/document.md --json
ixf docs chunk /tmp/ixf-docs/document.md --index 0
ixf docs cleanup /tmp/ixf-docs
```

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, document IDs, or generated private content unless needed for the user's requested analysis.
