# Bitable API Apply Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add API-first `ixf bitable record create --apply` support for text and attachment fields while keeping existing-record `attach --apply` fail-closed until its update contract is captured.

**Architecture:** Keep all runtime behavior in Go under `internal/bitable`. Reuse the existing URL resolver, clientvars parser, dry-run planner, cookie/session handling, and docx image upload patterns where they match captured bitable contracts. Do not use browser UI automation in the CLI; browser/CDP captures are diagnostic evidence only.

**Tech Stack:** Go standard library, existing `cmd/ixf` CLI dispatch, existing `internal/bitable` metadata parser, local `httptest` API fixtures, i讯飞 bitable private APIs captured from authenticated browser traffic.

## Global Constraints

- Go-only runtime; do not add Python fallback.
- Do not call `ixfdoc` or `ixfwrite`.
- Do not implement CLI writes through browser UI automation.
- Keep `--dry-run` mutation-free.
- Require explicit `--apply` for remote mutation.
- Support text and attachment fields first; fail closed for unsupported field types.
- Redact cookie values, CSRF tokens, full private tokens, raw request payloads, and full private URLs from command output.
- Apply must verify by refetching clientvars and checking required text plus attachment names.

---

### Task 1: Capture Record Mutation API Contract

**Files:**
- Create: `docs/superpowers/specs/2026-08-14-bitable-api-apply-contract.md`

**Interfaces:**
- Consumes: authenticated browser diagnostic traffic from `/tmp/ixf-bitable-write-capture.json`
- Produces: documented request sequence for record creation and attachment binding

- [ ] **Step 1: Capture WebSocket frames for one UI record submit**

Run a one-off CDP diagnostic that listens to `Network.webSocketFrameSent`, `Network.webSocketFrameReceived`, `Network.requestWillBeSent`, and `Network.responseReceived`. Submit one test row in the known test bitable with text plus `ceph_logo.jpeg`.

Expected evidence:
- attachment upload calls include `box/upload/prepare`, `box/stream/upload/merge_block`, and `box/upload/finish`
- record field writes appear in WebSocket frames or a late XHR/fetch endpoint
- captured payload includes a record id, table id, field ids, text values, and attachment token metadata

- [ ] **Step 2: Write the contract note**

Document only the shape needed by Go implementation:

```markdown
# Bitable API Apply Contract

## Upload

- `POST /space/api/box/upload/prepare/`
- `POST /space/api/box/stream/upload/merge_block/?upload_id=<id>&mount_point=bitable_image`
- `POST /space/api/box/upload/finish/`

## Record Mutation

- Transport: <captured transport>
- Request body fields: <captured minimal field list>
- Response success condition: <captured success condition>

## Verification

- Refetch `/space/api/v1/bitable/<base>/clientvars`.
- Decode `oldSchema.gzipSchema`.
- Locate expected text and attachment file names in `recordMap`.
```

- [ ] **Step 3: Self-check the contract**

Confirm the contract contains no cookie values, CSRF values, full base tokens, full record ids, full file tokens, or raw binary upload body.

---

### Task 2: Add Failing Apply Tests

**Files:**
- Modify: `internal/bitable/bitable_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`

**Interfaces:**
- Consumes: existing `RecordCreate`, `Attach`, fixture clientvars, and CLI command flags
- Produces: failing tests for API-first apply behavior

- [ ] **Step 1: Replace apply-gate unit tests with expected apply behavior**

Change `TestRecordCreateApplyFailsUntilContractCaptured` into `TestRecordCreateApplyUploadsAttachmentsWritesRecordAndVerifies`. The fixture server should:

```go
case "/space/api/v1/bitable/bas_fixture/clientvars":
    // First call returns 2 records. Second call returns 3 records with text and attachment.
case "/space/api/box/upload/prepare/":
    // Assert mount_point=bitable_image and mount_node_token=bas_fixture.
case "/space/api/box/stream/upload/merge_block/":
    // Assert mount_point=bitable_image and body is non-empty.
case "/space/api/box/upload/finish/":
    // Return a file token.
case "/space/api/<captured-record-mutation-path>":
    // Assert text and attachment metadata are present.
```

- [ ] **Step 2: Add unsupported field fail-closed test**

Add a test proving `record create --apply` rejects unsupported non-text/non-attachment fields:

```go
if err == nil || !strings.Contains(err.Error(), "unsupported apply field type") {
    t.Fatalf("unsupported field error = %v", err)
}
```

- [ ] **Step 3: Add CLI integration test**

Add a CLI fixture for:

```bash
ixf bitable record create \
  --url "$server/base/bas_fixture?table=tbl_main&view=vew_grid" \
  --input row.json \
  --cookies cookies.json \
  --space-api "$server" \
  --apply \
  --json
```

Expected JSON:

```json
{
  "ok": true,
  "dryRun": false,
  "applied": true,
  "uploadedFileCount": 1,
  "verify": { "ok": true }
}
```

- [ ] **Step 4: Run tests and verify RED**

Run:

```bash
go test ./internal/bitable -run 'RecordCreateApply|unsupported apply' -count=1
go test ./cmd/ixf -run CLIBitableRecordCreate -count=1
```

Expected: FAIL because apply still returns the captured-contract gate error.

---

### Task 3: Implement Upload Client

**Files:**
- Modify: `internal/bitable/bitable.go`

**Interfaces:**
- Consumes: `session`, `fileMetadata`, attachment local paths
- Produces: `uploadBitableFile(source Source, filePath string, file fileMetadata) (bitableUploadedFile, error)`

- [ ] **Step 1: Add upload result types**

Add:

```go
type bitableUploadedFile struct {
    Token string
    Name string
    MimeType string
    Size int64
    Timestamp int64
}
```

- [ ] **Step 2: Implement prepare / merge / finish**

Use `session.addHeaders` and JSON requests for prepare/finish. Use raw file bytes for merge block. Mount point must be `bitable_image` and mount node token must be the base token unless the captured contract requires a narrower node token.

- [ ] **Step 3: Run upload tests and verify GREEN for upload layer**

Run:

```bash
go test ./internal/bitable -run RecordCreateApply -count=1
```

Expected: test proceeds past upload assertions and fails at the not-yet-implemented record mutation call.

---

### Task 4: Implement Record Mutation Client

**Files:**
- Modify: `internal/bitable/bitable.go`

**Interfaces:**
- Consumes: captured mutation contract from Task 1, planned fields, uploaded file metadata
- Produces: `writeRecordCreate(source Source, meta Metadata, fields map[string]any, uploads map[string][]bitableUploadedFile) (string, error)`

- [ ] **Step 1: Build cell data for supported fields**

Text field values should become the captured rich-text shape. Attachment values should include:

```go
map[string]any{
    "attachmentToken": upload.Token,
    "id": upload.Token,
    "name": upload.Name,
    "mimeType": upload.MimeType,
    "size": upload.Size,
    "timeStamp": upload.Timestamp,
}
```

- [ ] **Step 2: Send the captured mutation request**

Implement the minimal request sequence needed by the captured API contract. Return the created record id when available.

- [ ] **Step 3: Fail closed for unsupported field types**

Only allow `text` and `attachment` in the first apply implementation. Return `unsupported apply field type "<type>" for field "<name>"` for single select, datetime, user, relation, formula, lookup, or unknown field types.

- [ ] **Step 4: Run mutation tests and verify GREEN**

Run:

```bash
go test ./internal/bitable -run RecordCreateApply -count=1
```

Expected: PASS.

---

### Task 5: Add Apply Verification and Keep Attach Gated

**Files:**
- Modify: `internal/bitable/bitable.go`
- Modify: `internal/bitable/bitable_test.go`

**Interfaces:**
- Consumes: `session.clientVars`, `ParseClientVars`, `matchingRecords`
- Produces: `verifyRecordCreate(...)` for new records and a precise `Attach --apply` gate for existing-record updates

- [ ] **Step 1: Implement record create verification**

After mutation, refetch clientvars and verify:
- the created record id exists when the API returns or generates one
- expected text appears in at least one record
- expected attachment names appear in the attachment field

- [ ] **Step 2: Keep `Attach --apply` fail-closed**

Existing-record attachment update is a separate API contract from add-record. Keep `Attach --apply` gated with a precise error until that record-update contract is captured.

- [ ] **Step 3: Run attach tests**

Run:

```bash
go test ./internal/bitable -run Attach -count=1
```

Expected: PASS for precise gated attach apply while record-update contract remains missing.

---

### Task 6: Update CLI Docs and Verify

**Files:**
- Modify: `README.md`
- Modify: `docs/go-python-parity.md`
- Modify: `docs/agent-routing.md`
- Modify: `skills/codex/using-ixf-toolbox/SKILL.md`
- Modify: `skills/claude-code/using-ixf-toolbox/SKILL.md`

**Interfaces:**
- Consumes: implemented apply behavior and limitations
- Produces: current user-facing guidance

- [ ] **Step 1: Update command documentation**

Replace "apply fails closed" language for `record create --apply` with the implemented text/attachment scope. Keep unsupported field limitations explicit.

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./...
go vet ./...
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 3: Run real smoke test on approved test bitable**

Run `ixf bitable record create --apply --json` against the approved test table using a unique test summary and `~/Downloads/ceph_logo.jpeg`. Verify with `ixf bitable inspect --json` and direct clientvars readback that the row and attachment exist.
