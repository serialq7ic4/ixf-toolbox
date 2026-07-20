# Docs Update Design

## Goal

Add a safe API-only ability to update an existing authorized docx document from
local Markdown while preserving the original document URL, permissions, and
location.

## Command Boundary

`ixf docs publish` keeps its existing create-only behavior. It creates a new
docx document from Markdown and never edits an existing docx.

`ixf docs update` is the new existing-document command. It targets one
authorized docx URL and updates that document. The first supported update mode is
`replace_body`: replace the document body's top-level content blocks with blocks
generated from the Markdown input.

## Safety Model

All write commands are dry-run-first. `ixf docs update` must not mutate remote
state unless `--apply` is present. Dry-run output must identify the target token,
operation, mode, destructive nature, current top-level block count, planned
top-level block count, and complex-block risk.

The command must reject ambiguous or unsupported targets. It accepts docx URLs
only. It must not support wiki URLs in the first update release because wiki
resolution adds an extra mutation target ambiguity.

The first apply release must reject complex existing content by default. Complex
content means any existing top-level subtree containing blocks outside the
supported Markdown writer set: page, text, heading blocks, bullet, ordered, code,
quote_container, and callout. A later stabilization release may add an explicit
override for known destructive replacement.

## API Model

The existing publisher already uses the required APIs for creation:

- `GET /space/api/docx/pages/client_vars?id=<doc>&open_type=1` to read block
  state and versions.
- `POST /space/api/docx/blocks/user_change` to submit a `change_map`.

`docs update` reuses the same authenticated session, CSRF handling, Markdown
parser, block factory, and verification path. Instead of calling document
creation first, it reads the target document state, builds a change map that
deletes current root children and inserts new root children, then writes the new
blocks.

## Version Plan

- `v3.8.1`: fix current docs writer boundary and contract tests so agents stop
  claiming existing-doc modification support before it exists.
- `v3.9.0`: add `ixf docs update` dry-run/preflight with no remote mutation.
- `v3.10.0`: add `ixf docs update --apply` for `replace_body`, with default
  complex-block rejection and post-write verification.
- `v3.11.0`: stabilize docs update diagnostics, docs, skills, and smoke runbook.

## Out of Scope

The first release train does not implement semantic diff editing, paragraph-level
patches, comments, permissions changes, moving documents, title changes, wiki
target resolution, or rich visual fidelity beyond the Markdown writer's existing
block set.
