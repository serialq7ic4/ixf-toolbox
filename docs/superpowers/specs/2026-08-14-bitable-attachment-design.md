# Bitable Attachment Upload Design

## Context

`ixf-toolbox` currently separates document, sheet, and Messenger operations by
data model:

- `ixf docs` handles docx block operations.
- `ixf sheets` handles direct spreadsheet links with A1 ranges and TSV values.
- Existing bitable support is read-oriented and can render wiki bitables as TSV.

The missing capability is uploading a local image or file into a bitable
attachment/image field. This is not a sheet cell update and not a docx image
block update. Even when the user starts from a docx or wiki link, the write
target is the bitable data layer: base, table, view, record, and field.

## Goals

- Add a dedicated `ixf bitable` command family.
- Accept direct bitable links, wiki bitable links, and docx links containing an
  embedded bitable view.
- Resolve the user-facing source into a concrete bitable target:
  `baseToken`, `tableId`, `viewId`, field metadata, and record IDs.
- Support a minimal attachment/image write path:
  upload one local file and bind it to one attachment-compatible field on one
  matched record.
- Preserve the existing dry-run-first and explicit-apply safety model.
- Avoid printing cookie values, CSRF tokens, raw private API payloads, private
  document IDs, or full private URLs.

## Non-Goals

- Do not merge this capability into `ixf sheets update`.
- Do not treat a bitable attachment write as a docx block patch.
- Do not implement arbitrary record editing in the first version.
- Do not support bulk upload or many-record fanout in the first version.
- Do not silently infer a record when a match is ambiguous.

## CLI Shape

Recommended commands:

```bash
ixf bitable inspect --url <docx-or-wiki-or-bitable-url> --json

ixf bitable read --url <docx-or-wiki-or-bitable-url> --json

ixf bitable attach \
  --url <docx-or-wiki-or-bitable-url> \
  --field <field-name-or-id> \
  --record-match "Field=Value" \
  --file <local-path> \
  --dry-run

ixf bitable attach ... --apply
```

`inspect` is the preflight command. It should report source kind, visible
tables/views, field names/types, record count, and whether attachment fields
exist.

`read` is optional for the first implementation if `inspect` exposes enough
metadata, but it is useful because bitable writes need stable record and field
identifiers instead of rendered TSV text.

`attach` is the first write command. It should accept exactly one local file and
one target field for one resolved record.

## Source Resolution

The bitable resolver should accept three source categories.

Direct bitable:

- Parse the base token from the URL.
- Fetch bitable client variables.
- Select a default table/view when the URL or base metadata identifies one.
- Require explicit `--table` or `--view` if there are multiple viable choices.

Wiki bitable:

- Fetch wiki HTML.
- Reuse the current wiki bitable token extraction path where possible.
- Resolve the wiki object token to a base token.
- Continue with the same bitable client variable path as direct bitable.

Docx embedded bitable:

- Fetch the docx block graph.
- Locate supported embedded bitable blocks such as `view` or `file`.
- Extract the underlying base/view metadata from the block payload.
- If the document contains multiple embedded bitables, require a selector such
  as `--embedded-index`, `--view`, or `--table`.
- Keep docx token and referer metadata only as source-resolution context; do not
  route the write through `ixf docs`.

The resolver should return a single normalized target structure:

```json
{
  "sourceKind": "docx_embedded_bitable",
  "baseToken": "<redacted>",
  "tableId": "tbl...",
  "viewId": "vew...",
  "refererKind": "docx"
}
```

## Attachment Write Flow

Dry-run:

1. Resolve the URL into one bitable target.
2. Fetch table/view/field/record metadata.
3. Resolve `--field` to exactly one attachment-compatible field.
4. Resolve `--record-match` or `--record-id` to exactly one record.
5. Validate the local file exists, is a regular file, has a supported MIME type,
   and is below any configured size limit.
6. Return a plan with `willUpload:true` and `willUpdateRecord:true`, but do not
   perform network mutation.

Apply:

1. Repeat the dry-run validations.
2. Upload the local file through the bitable-compatible file endpoint.
3. Update the target record field with the returned file token and metadata.
4. Re-fetch the record and verify the field contains the uploaded file name,
   token, MIME type, or other stable returned identifier.

The docx Mermaid upload path can inform multipart upload mechanics, but the
bitable implementation must not reuse `docx_image` mount semantics unless the
captured bitable API contract explicitly requires it.

## Safety Model

- `--apply` is required for mutation.
- `--dry-run` and `--apply` are mutually exclusive.
- Ambiguous table, view, field, or record resolution fails closed.
- Output redacts tokens and private URLs by default.
- Verification failure makes the command fail even if the upload endpoint
  returned success.
- Existing docx/wiki content must not be patched or replaced as part of a
  bitable attachment write.

## JSON Output

Dry-run output should include enough information for the agent and user to
confirm the target without leaking secrets:

```json
{
  "ok": true,
  "dryRun": true,
  "operation": "bitable_attach",
  "sourceKind": "docx_embedded_bitable",
  "target": {
    "baseTokenPrefix": "bas",
    "tableId": "tbl...",
    "viewId": "vew..."
  },
  "recordMatchCount": 1,
  "field": {
    "name": "Screenshot",
    "id": "fld...",
    "type": "attachment"
  },
  "file": {
    "name": "ceph_logo.jpeg",
    "mimeType": "image/jpeg",
    "sizeBytes": 6750
  },
  "willUpload": true,
  "willUpdateRecord": true
}
```

Apply output should add upload and verification summaries:

```json
{
  "ok": true,
  "dryRun": false,
  "applied": true,
  "uploadedFileCount": 1,
  "verify": {
    "ok": true,
    "recordMatched": true,
    "fieldContainsUploadedFile": true
  }
}
```

## Implementation Boundaries

Suggested packages:

- `internal/bitable`: resolver, metadata rendering, upload planning, attach
  apply, and verification.
- `cmd/ixf`: command routing for `ixf bitable inspect/read/attach`.
- `internal/docslocal`: keep existing read conversion helpers, but avoid turning
  bitable write logic into docx read logic.
- Shared secret-safe URL and token redaction helpers should be reused or
  extracted only if needed.

The implementation should first add local tests using captured/synthetic
clientvars payloads, then add CLI integration tests with a local HTTP fixture
that verifies request shape without touching real tenant data.

## Test Strategy

- Resolver tests:
  - direct bitable URL resolves to one base target;
  - wiki bitable HTML resolves to one base target;
  - docx embedded bitable view resolves to one base target;
  - multiple embedded bitables require an explicit selector.
- Dry-run tests:
  - refuses missing file;
  - refuses non-attachment fields;
  - refuses ambiguous record matches;
  - reports upload and record-update plan without mutation.
- Apply tests:
  - uploads the file to the expected endpoint;
  - updates the expected record field payload;
  - verifies by re-reading the record;
  - fails if verification cannot find the uploaded file.
- Privacy tests:
  - no cookie values, CSRF tokens, full document IDs, or raw private URLs appear
    in JSON or text output.

## Open Implementation Detail

The exact bitable upload endpoint and record update payload must be captured
from the web client or existing API behavior before apply support is enabled.
Until that contract is captured, `ixf bitable attach --apply` should return a
clear unsupported-contract error while `inspect` and `attach --dry-run` can
still be implemented safely.
