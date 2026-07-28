# Docs Structure Preflight Design

## Goal

Add a safe document-structure preflight that runs before document read and write
workflows. The structure model must expose heading layout, section boundaries,
block type counts, complex-block risk, and duplicate-heading diagnostics without
printing private tokens, raw block IDs, cookies, CSRF tokens, or raw API payloads.

The intent is not only a standalone diagnostic command. Structure analysis is a
required foundation for reading, patching, and whole-body update planning because
agents need to understand a document's shape before summarizing or mutating it.

## Command Surface

Add a read-only command:

```bash
ixf docs structure https://tenant.example.test/wiki/example --json
```

The command accepts direct docx URLs and wiki URLs that resolve to a backing docx
page. It rejects unsupported cloud-document types using the existing docs read
routing boundaries.

The JSON output must be secret-safe and include:

- `operation:"docs_structure"`
- `targetKind`
- redacted target token metadata, not the full token
- title
- top-level block count
- block type counts
- heading paths with level, root index, and section range
- previous/next top-level sibling summaries
- complex block count and sorted complex block types
- duplicate heading diagnostics

## Read Preflight

`ixf docs read --out-dir <dir> --print-manifest` should include the same safe
structure summary for every supported remote docx or wiki-backed docx result.
The structure data lives inside `manifest.json` and is also written as
`<artifact>.structure.json` for durable follow-up by agents.

Local Markdown, direct sheets, bitable, and mindnote reads do not need docx block
structure. Their manifest entries should keep `structure:null` or omit the
structure file.

## Write Preflight

All existing-docx write dry-runs must include the same safe structure summary:

- `ixf docs update --dry-run`
- `ixf docs patch insert --dry-run`
- `ixf docs patch replace-section --dry-run`
- `ixf docs patch delete-section --dry-run`

Apply flows may also include structure metadata, but dry-run is the contract that
agents must inspect before remote mutation. Existing mutation APIs and payload
formats should not change in v3.21.

## Safety Rules

- Do not print full private document tokens or block IDs in the structure output.
- Do not print cookies, CSRF tokens, raw private API payloads, or full private
  URLs.
- Block IDs should be represented only by stable redacted labels, such as a short
  prefix plus length.
- Text snippets should be short and whitespace-normalized.
- The command is read-only and must not call the document write endpoint.

## Out Of Scope

This release does not add `move-section`, `replace-block`, arbitrary Markdown
semantic diffing, visual diffing, or rich-block serialization. The goal is to
make future precise writes safer by exposing the structure that patch operations
already consume internally.
