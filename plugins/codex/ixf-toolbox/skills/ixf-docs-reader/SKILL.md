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
4. When the user points at a wiki directory instead of one document, or asks
   which documents live under a node, list it before reading:
   `ixf docs tree "<wiki-url>" --json`
5. Read other sources into a temporary output directory and inspect the manifest structure summary when it affects the analysis:
   `ixf docs read "<source>" --out-dir <dir> --print-manifest`
6. For embedded sheets inside a docx, use:
   `ixf docs read "<source>" --out-dir <dir> --expand-sheets --print-manifest`
7. Analyze generated Markdown/TSV artifacts.
8. Do not commit private artifacts unless the user explicitly asks.

## Commands

```bash
ixf docs inspect "<source>" --json
ixf docs structure "<doc-or-wiki-url>" --json
ixf docs tree "<wiki-url>" --json
ixf docs tree "<wiki-url>" --max-depth 3
ixf sheets read "<direct-sheets-link>"
ixf docs read "<source>" --out-dir /tmp/ixf-docs --print-manifest
ixf docs read "<source>" --out-dir /tmp/ixf-docs --expand-sheets --print-manifest
ixf docs outline /tmp/ixf-docs/document.md --json
ixf docs chunk /tmp/ixf-docs/document.md --index 0
ixf docs cleanup /tmp/ixf-docs
```

## Directory Listing Boundary

Use `ixf docs tree` to answer "what documents are under this directory" before
reading anything. It is read-only and has no `--apply`.

- It accepts a `/wiki/<token>` node URL only. Drive folder URLs
  (`/drive/folder/...`) are out of scope and are rejected; ask for the wiki node
  or space URL rather than guessing a document.
- It lists direct children only by default: one level, one request. Pass
  `--max-depth N` to walk N levels below the node, so `--max-depth 1` is the
  default. A deeper walk costs one request per node that has children, with no
  retry or rate limiting, and stops at an internal 500-node cap.
- A text listing ending with a line starting `INCOMPLETE`, or JSON with
  `truncated:true`, means nodes are missing. Report it as a partial listing.
- A node's kind comes from its object-token prefix: `dox` is a docx document,
  `sht` a native sheet, `box` an uploaded binary file such as an `.xlsx`
  attachment, `bas` a bitable, and `bmn` a mindnote.
- `ixf docs read` reads docx and bitable nodes. A wiki-hosted native sheet and an
  uploaded file both fail. Do not pipe every listed URL into `ixf docs read`; read
  the nodes reported `readable:true` and tell the user what was skipped. Reading a
  wiki-hosted native sheet needs a separate sheet-id lookup that this command does
  not perform. A mindnote is reported unreadable: a wiki URL cannot reach the
  mindnote read path, which is reasoned from that path rather than live-confirmed.
- A child whose detail the current session cannot see is omitted, so the listing
  is what is reachable now, not a guaranteed complete inventory.
- Because the default is one level, the kinds you see are the ones among the
  direct children, not every kind in the subtree. A readable bitable nested
  deeper appears only when `--max-depth` reaches it.

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, document IDs, or generated private content unless needed for the user's requested analysis.
