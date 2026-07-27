# Docs Block Patch Design

## Goal

Add API-only block-level document patching for authorized i讯飞/LarkShell docx
and wiki-backed docx pages. The first supported operation is inserting new
Markdown-generated content, especially native tables, into a specific section
without replacing the whole document body.

The design exists because `ixf docs update` intentionally uses `replace_body`.
That mode is useful for confirmed whole-document rewrites, but it is too broad
for requests such as "insert this table under a heading". Rebuilding the entire
body from Markdown can damage rich blocks that Markdown cannot represent
losslessly, such as high-light/callout variants, images, embedded sheets, app
blocks, and other visual formatting.

## Current Boundary

`ixf docs publish` remains create-only.

`ixf docs update` remains the destructive whole-body replacement command. It
keeps the existing URL, permissions, and location, but replaces root body
children with blocks generated from a complete Markdown document. This command
must continue to show `mode:replace_body` and `destructive:true`.

Block patching must be a separate command family so agents cannot accidentally
use a destructive full-body replace for a localized insertion request.

## Recommended Command Surface

Introduce a `docs patch` family, starting with insert:

```bash
ixf docs patch insert table.md \
  --url https://tenant.example.test/wiki/example \
  --under-heading "1.1 Account initialization" \
  --dry-run
```

Apply requires explicit confirmation:

```bash
ixf docs patch insert table.md \
  --url https://tenant.example.test/wiki/example \
  --under-heading "1.1 Account initialization" \
  --apply
```

The command accepts direct docx URLs and wiki URLs that resolve to a backing docx
page. It rejects non-docx wiki suites, direct sheets, bitables, mindnotes, and
unsupported app documents. Sheet cell changes stay on `ixf sheets update`.

Initial positioning flags:

- `--under-heading <text>`: locate a top-level heading by normalized text.
- `--position section-end`: default; insert before the next heading whose level
  is less than or equal to the matched heading level.
- `--position after-heading`: insert immediately after the matched heading.

Later releases can add `--before-heading`, `--after-block-id`, and
`--before-block-id` after the structured locator model is stable.

## Patch Model

The patch engine must operate on the remote block graph, not on rendered
Markdown for existing content.

1. Fetch target `client_vars` and build a document graph from `block_map`.
2. Resolve the target URL. For wiki URLs, resolve the backing docx token and use
   that docx page as the mutation target.
3. Locate the anchor heading in root body children.
4. Compute the insertion index from the selected position.
5. Parse the input Markdown as a fragment, not as a full document replacement.
6. Generate new docx blocks for the fragment using the existing block factory.
7. Submit a `user_change` change map that only:
   - adds new block objects with `oi`;
   - inserts new top-level block IDs into the root `children` array with `li`.
8. Do not submit `ld` or `od` operations for any existing block during insert.
9. Refetch and verify the result.

This preserves all old block IDs and all old block data. Existing rich blocks are
not rendered to Markdown and are not regenerated.

## Fragment Markdown Rules

The insert input is a Markdown fragment. It does not need to start with a level-1
title. Supported fragment blocks in the first release:

- paragraphs
- headings
- bullet and ordered lists
- code blocks
- quote containers
- callouts already supported by the writer
- native docx tables generated from Markdown tables

Unsupported fragment syntax must fail dry-run instead of falling back to empty
or lossy blocks. Markdown tables must report `tableBlockType:"table"` and
`tableFallbackCount:0`, matching the current native table contract.

## Locator Model

Heading matching uses normalized text:

- trim whitespace;
- collapse repeated whitespace;
- normalize full-width and half-width spaces;
- compare exact text first;
- optionally support case-insensitive ASCII matching for English headings.

If zero headings match, dry-run fails with a secret-safe list of nearby headings.
If multiple headings match, dry-run fails and asks the user to disambiguate by
using a more specific heading path once that feature exists.

The first release does not support nested heading paths. A later release should
add:

```bash
--heading-path "Parent > Child > 1.1 Account initialization"
```

That path should be resolved against the root heading hierarchy derived from
heading levels.

## Dry-Run Output

`docs patch insert --dry-run` must not mutate remote state. It should return JSON
with at least:

- `ok:true`
- `operation:"docs_patch_insert"`
- `mode:"block_insert"`
- `destructive:false`
- `willWrite:false`
- `targetKind:"docx"` or `targetKind:"wiki_docx"`
- `targetTokenPrefix` and token length, not full private tokens
- `anchorHeading`
- `anchorHeadingLevel`
- `position`
- `insertIndex`
- `currentTopLevelBlocks`
- `plannedInsertedTopLevelBlocks`
- `plannedNewBlockCount`
- `tableCount`
- `tableBlockType`
- `tableFallbackCount`
- `existingBlocksTouched:false`

Dry-run should also show the previous and next sibling block summaries around
the insertion point, using text snippets rather than raw IDs.

## Apply Verification

Apply must refetch target state before writing, recompute the anchor, and submit
the patch against the latest root version.

After write, verification must refetch the document and assert:

- all pre-existing block IDs still exist;
- all pre-existing root children remain in the same relative order;
- no old block subtree data changed, ignoring version metadata that the service
  may update;
- inserted block IDs appear at the expected root child index;
- required text from the inserted fragment is present;
- `tableFallbackCount` is zero for Markdown table inserts.

Apply output must include:

- `existingBlocksTouched:false` when all old block signatures are unchanged;
- `insertedTopLevelBlocks`;
- `insertedBlockCount`;
- `verify.ok`;
- `verify.unchangedExistingBlocks`;
- `verify.missingRequiredText`;
- `url`.

If verification detects changed old blocks, the command must report failure even
if the HTTP write succeeded.

## Concurrency

Dry-run is advisory. Apply always refetches and recomputes the locator so it does
not blindly reuse stale insertion indexes. If the heading disappears, becomes
ambiguous, or moves under an unsupported container, apply fails before writing.

The root page version from the refetched state must be included in the
`user_change` payload. If the service rejects the change due to version conflict,
the CLI reports a retryable concurrency error with no private payload dump.

## Safety Model

Block insert is non-destructive by design. It must not require
`--allow-complex-replace` because complex existing blocks are preserved.

The command still requires explicit `--apply` for remote mutation. Agents must
show the dry-run plan first for user-visible writes.

The CLI must not print cookies, CSRF tokens, full document tokens, raw private
API payloads, or generated private artifacts. Examples in repository docs must
use placeholder tenant URLs.

## Future Patch Operations

### Replace Section

Add:

```bash
ixf docs patch replace-section section.md \
  --url https://tenant.example.test/docx/example \
  --under-heading "1.1 Account initialization" \
  --dry-run
```

This removes only root children in the matched section range and inserts new
fragment blocks in their place. It is destructive inside the bounded section but
must preserve everything outside the section.

Default behavior:

- reject if the target section contains complex blocks;
- require `--allow-complex-section-replace` to replace a section containing rich
  blocks;
- verify all outside-section block signatures remain unchanged.

### Delete Section

Add a confirmed deletion operation for a matched section:

```bash
ixf docs patch delete-section \
  --url https://tenant.example.test/docx/example \
  --under-heading "Deprecated section" \
  --dry-run
```

Deletion must be bounded to root children in the section range and must reject
complex content unless explicitly allowed.

### Move Section

Add a move operation only after insert and bounded replacement are stable.
Moving should reuse existing block IDs by deleting and reinserting root child
references, not by regenerating block data.

### Replace Block

Add block-ID based replacement only for advanced workflows where a previous
`docs structure` or patch dry-run has identified a stable block. This should not
be the default user-facing path because raw block IDs are not natural in agent
prompts.

## Structured Read Support

The reader already consumes `block_map`, `children`, `parent_id`, block types,
and raw data internally. The patch engine should share that graph builder instead
of re-parsing rendered Markdown.

Add an internal `docxgraph` package or exported docx graph type with:

- root page metadata;
- ordered root children;
- block lookup by ID;
- heading hierarchy;
- subtree signature calculation;
- section range calculation;
- secret-safe block summaries for dry-run output.

An optional later command can expose safe structure diagnostics:

```bash
ixf docs structure https://tenant.example.test/wiki/example --json
```

That command should redact full IDs by default and should not be required for
normal insert workflows.

## Version Plan

- `v3.16.0`: add shared docx graph/section locator foundation and fixture tests.
  No remote mutation surface changes.
- `v3.17.0`: add `ixf docs patch insert --dry-run` for docx and wiki-backed
  docx targets.
- `v3.18.0`: add `ixf docs patch insert --apply` with unchanged-block
  verification and native table insertion smoke coverage.
- `v3.19.0`: update README, routing docs, and installed skills so natural
  "insert under heading" requests use block patch insert, not `docs update`.
- `v3.20.0`: add bounded `replace-section` and `delete-section` dry-run/apply
  with section-only destructive safeguards.
- `v3.21.0+`: evaluate move-section, block-ID replacement, and richer locator
  syntax after live patch usage shows stable API behavior.

Patch releases can be inserted between these milestones for server-contract
diagnostics or verification hardening.

## Testing

Unit tests:

- heading normalization and duplicate/missing heading errors;
- section range calculation for heading levels;
- insert index calculation for `section-end` and `after-heading`;
- generated change map contains `li` and `oi` only for insert;
- generated change map contains no `ld` or `od` for existing blocks;
- subtree signature ignores service-managed version fields but catches content
  and children changes;
- Markdown table fragments generate native table blocks.

CLI integration tests with mocked endpoints:

- wiki URL resolves to backing docx token before patching;
- dry-run reports non-destructive insert metadata;
- apply inserts a table under the requested heading;
- apply verifies old rich blocks remain unchanged;
- ambiguous heading fails without writing;
- direct sheets, bitable, and unsupported wiki suites are rejected.

Manual smoke tests:

- use a non-sensitive test wiki/docx containing a heading, a high-light/callout
  block, and a table insertion target;
- run dry-run and inspect insertion point;
- apply only after confirmation;
- read back and confirm the inserted table appears while the pre-existing rich
  block remains visually intact.

## Agent Routing Changes

Once insert apply is available, writer skills must route localized insertion
requests to `ixf docs patch insert`, not `ixf docs update`.

Examples that should use patch insert:

- "insert this table under heading X"
- "add this paragraph after section Y"
- "append this checklist to chapter Z"

Examples that may still use `docs update`:

- "rewrite the whole document from this Markdown"
- "replace the entire body"
- "I accept losing unsupported rich formatting and want a full rebuild"

When intent is ambiguous, agents must default to non-destructive patch planning
or ask for confirmation rather than choosing `replace_body`.

## Out of Scope

The initial insert release does not implement comments, permissions, document
movement, title rename, embedded sheet cell edits, bitable edits, visual diff UI,
or arbitrary semantic Markdown-to-document diffing.

It also does not promise lossless Markdown export of all existing rich blocks.
The safety guarantee comes from not touching existing blocks during localized
patches, not from being able to serialize every block type to Markdown.
