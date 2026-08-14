# Bitable Attachment Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add a dedicated `ixf bitable` command family that can inspect/read bitable targets and safely plan image/attachment uploads from direct bitable, wiki bitable, or docx embedded bitable URLs.

**Architecture:** Add a focused `internal/bitable` package with URL/source resolution, clientvars parsing, metadata rendering, attachment dry-run planning, and apply contract gating. Keep docx/wiki only as source resolvers; record and attachment semantics stay in `bitable`, not `docs` or `sheets`.

**Tech Stack:** Go standard library, existing `docslocal`/`docxgraph` parsing patterns, existing CLI dispatch in `cmd/ixf/main.go`, local HTTP fixtures for integration tests.

## Global Constraints

- Go-only runtime; do not add Python fallback.
- Do not call `ixfdoc` or `ixfwrite`.
- Do not merge bitable writes into `ixf sheets update`.
- Do not route bitable attachment writes through `ixf docs update` or docx block patch commands.
- Dry-run must not perform network mutation.
- Apply must fail closed until the exact bitable upload and record-update contract is captured.
- Output must not print cookie values, CSRF tokens, raw private API payloads, full private URLs, or full private document/base tokens.

---

### Task 1: Bitable Metadata Parser

**Files:**
- Create: `internal/bitable/bitable.go`
- Create: `internal/bitable/bitable_test.go`

**Interfaces:**
- Produces: `type Field`, `type Record`, `type Metadata`, `ParseClientVars(data map[string]any, baseToken string) (Metadata, error)`, `RenderValue(value any, field Field, userMap map[string]string, tzName string) string`
- Consumes: synthetic bitable clientvars payloads matching `internal/docslocal/bitable.go`

- [x] **Step 1: Write failing parser tests**

Add tests for one table, one grid view, fields, records, attachment field detection, and row rendering:

```go
func TestParseClientVarsReturnsTablesViewsFieldsAndRecords(t *testing.T) {
    data := bitableClientVarsFixture(t)
    meta, err := ParseClientVars(data, "bas_fixture")
    if err != nil {
        t.Fatalf("ParseClientVars returned error: %v", err)
    }
    if meta.BaseToken != "bas_fixture" || meta.Title != "Bug Tracker" {
        t.Fatalf("metadata identity = %+v", meta)
    }
    if len(meta.Tables) != 1 || meta.Tables[0].ID != "tbl_main" || meta.Tables[0].Name != "Issues" {
        t.Fatalf("tables = %+v", meta.Tables)
    }
    if len(meta.Views) != 1 || meta.Views[0].ID != "vew_grid" || meta.Views[0].Name != "Grid" {
        t.Fatalf("views = %+v", meta.Views)
    }
    if got := meta.FieldByName("Screenshot"); got == nil || !got.AttachmentCompatible {
        t.Fatalf("Screenshot field = %+v, want attachment-compatible", got)
    }
    if len(meta.Records) != 2 || meta.Records[0].ID != "rec_1" {
        t.Fatalf("records = %+v", meta.Records)
    }
}
```

- [x] **Step 2: Run parser tests and verify RED**

Run: `go test ./internal/bitable -run TestParseClientVarsReturnsTablesViewsFieldsAndRecords -count=1`

Expected: FAIL because `internal/bitable` and `ParseClientVars` do not exist.

- [x] **Step 3: Implement minimal parser**

Create `internal/bitable/bitable.go` with:

```go
package bitable

type Field struct {
    ID string
    Name string
    Type int
    TypeName string
    AttachmentCompatible bool
}

type Table struct { ID string; Name string }
type View struct { ID string; Name string; TableID string; Type int }
type Record struct { ID string; Values map[string]string; Raw map[string]any }
type Metadata struct {
    BaseToken string
    Title string
    Tables []Table
    Views []View
    Fields []Field
    Records []Record
}

func ParseClientVars(data map[string]any, baseToken string) (Metadata, error) { ... }
func (m Metadata) FieldByName(name string) *Field { ... }
```

Reuse the gzip JSON, `asMap`, `asSlice`, and value rendering style already present in `internal/docslocal/bitable.go`, copied into `internal/bitable` as private helpers.

- [x] **Step 4: Run parser tests and verify GREEN**

Run: `go test ./internal/bitable -run TestParseClientVarsReturnsTablesViewsFieldsAndRecords -count=1`

Expected: PASS.

---

### Task 2: Source Resolver and Inspect Payload

**Files:**
- Modify: `internal/bitable/bitable.go`
- Modify: `internal/bitable/bitable_test.go`

**Interfaces:**
- Consumes: `ParseClientVars`
- Produces: `type InspectConfig`, `func Inspect(config InspectConfig) (map[string]any, error)`, `func ParseSource(rawURL string) (Source, error)`

- [x] **Step 1: Write failing resolver tests**

Add tests:

```go
func TestParseSourceRecognizesDirectBitableURL(t *testing.T) {
    source, err := ParseSource("https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid")
    if err != nil {
        t.Fatalf("ParseSource returned error: %v", err)
    }
    if source.Kind != "direct_bitable" || source.BaseToken != "bas_fixture" || source.TableID != "tbl_main" || source.ViewID != "vew_grid" {
        t.Fatalf("source = %+v", source)
    }
}

func TestInspectRedactsTokensAndReportsAttachmentFields(t *testing.T) {
    payload, err := inspectFixture()
    if err != nil {
        t.Fatalf("Inspect returned error: %v", err)
    }
    if payload["ok"] != true || payload["operation"] != "bitable_inspect" {
        t.Fatalf("payload = %+v", payload)
    }
    if containsPrivateToken(payload, "bas_fixture") {
        t.Fatalf("inspect payload leaked base token: %+v", payload)
    }
}
```

- [x] **Step 2: Run resolver tests and verify RED**

Run: `go test ./internal/bitable -run 'ParseSource|InspectRedacts' -count=1`

Expected: FAIL because `ParseSource` and `Inspect` do not exist.

- [x] **Step 3: Implement source parsing and inspect payload**

Implement direct source parsing for `/base/<token>` and safe inspect payload construction. Include extension points for `wiki_bitable` and `docx_embedded_bitable`, but return clear unsupported resolver errors until network fetching is wired:

```go
type Source struct {
    Kind string
    RawURL string
    BaseURL string
    BaseToken string
    TableID string
    ViewID string
}

type InspectConfig struct {
    URL string
    ClientVars map[string]any
}
```

`Inspect` uses `ClientVars` when provided, enabling deterministic local tests before adding HTTP fetchers.

- [x] **Step 4: Run resolver tests and verify GREEN**

Run: `go test ./internal/bitable -run 'ParseSource|InspectRedacts' -count=1`

Expected: PASS.

---

### Task 3: Attachment Dry-Run Planner

**Files:**
- Modify: `internal/bitable/bitable.go`
- Modify: `internal/bitable/bitable_test.go`

**Interfaces:**
- Consumes: `InspectConfig`, `ParseClientVars`, `Metadata.FieldByName`
- Produces: `type AttachConfig`, `func Attach(config AttachConfig) (map[string]any, error)`

- [x] **Step 1: Write failing dry-run tests**

Add tests:

```go
func TestAttachDryRunPlansOneUploadWithoutMutation(t *testing.T) {
    file := writeFixtureFile(t, "ceph_logo.jpeg", []byte{0xff, 0xd8, 0xff, 0xd9})
    payload, err := Attach(AttachConfig{
        URL: "https://tenant.example/base/bas_fixture?table=tbl_main&view=vew_grid",
        Field: "Screenshot",
        RecordMatch: "Title=Image bug",
        FilePath: file,
        DryRun: true,
        ClientVars: bitableClientVarsFixture(t),
    })
    if err != nil {
        t.Fatalf("Attach dry-run returned error: %v", err)
    }
    if payload["ok"] != true || payload["dryRun"] != true || payload["willUpload"] != true || payload["willUpdateRecord"] != true {
        t.Fatalf("payload = %+v", payload)
    }
}

func TestAttachDryRunRejectsAmbiguousRecordMatch(t *testing.T) { ... }
func TestAttachDryRunRejectsNonAttachmentField(t *testing.T) { ... }
func TestAttachApplyFailsUntilContractCaptured(t *testing.T) { ... }
```

- [x] **Step 2: Run dry-run tests and verify RED**

Run: `go test ./internal/bitable -run 'AttachDryRun|AttachApply' -count=1`

Expected: FAIL because `Attach` and `AttachConfig` do not exist.

- [x] **Step 3: Implement dry-run planner and apply gate**

Implement:

```go
type AttachConfig struct {
    URL string
    Field string
    RecordID string
    RecordMatch string
    FilePath string
    DryRun bool
    Apply bool
    ClientVars map[string]any
}
```

Validation rules:

- `--dry-run` and `--apply` are mutually exclusive.
- Missing `--file`, `--field`, or record selector fails.
- `--record-match` uses `Field=Value` and must match exactly one rendered record.
- Field must be attachment-compatible.
- Local file must exist and be a regular file.
- `Apply:true` returns `bitable attach --apply is not available until the bitable upload API contract is captured`.

- [x] **Step 4: Run dry-run tests and verify GREEN**

Run: `go test ./internal/bitable -run 'AttachDryRun|AttachApply' -count=1`

Expected: PASS.

---

### Task 4: CLI Command Wiring

**Files:**
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`

**Interfaces:**
- Consumes: `bitable.Inspect`, `bitable.Attach`
- Produces: `ixf bitable inspect`, `ixf bitable read`, `ixf bitable attach`

- [x] **Step 1: Write failing CLI tests**

Add tests that verify help routing and JSON dry-run:

```go
func TestBitableCommandHelpListsInspectReadAttach(t *testing.T) {
    stdout, stderr, code := runCLITest(t, "bitable", "--help")
    if code != 0 || stderr != "" {
        t.Fatalf("help code=%d stderr=%q stdout=%q", code, stderr, stdout)
    }
    for _, expected := range []string{"inspect", "read", "attach"} {
        if !strings.Contains(stdout, expected) {
            t.Fatalf("help missing %q: %s", expected, stdout)
        }
    }
}
```

Add a local fixture CLI test for `attach --dry-run --json` through a test seam if needed.

- [x] **Step 2: Run CLI tests and verify RED**

Run: `go test ./cmd/ixf -run 'BitableCommand|BitableAttach' -count=1`

Expected: FAIL because `ixf bitable` is not routed.

- [x] **Step 3: Implement CLI route**

Add `runBitable`, `runBitableInspect`, `runBitableRead`, and `runBitableAttach` in `cmd/ixf/main.go`.

Root help should include:

```go
{"bitable", "Inspect or plan approved bitable attachment changes."}
```

`attach` flags:

- `--url`
- `--field`
- `--record-id`
- `--record-match`
- `--file`
- `--dry-run`
- `--apply`
- `--json`
- `--cookies`
- `--space-api`

- [x] **Step 4: Run CLI tests and verify GREEN**

Run: `go test ./cmd/ixf -run 'BitableCommand|BitableAttach' -count=1`

Expected: PASS.

---

### Task 5: Network Resolver Skeleton

**Files:**
- Modify: `internal/bitable/bitable.go`
- Modify: `internal/bitable/bitable_test.go`
- Modify: `internal/docslocal/bitable.go` only if shared exported helpers are needed

**Interfaces:**
- Consumes: `ParseSource`, `ParseClientVars`
- Produces: HTTP-backed inspect for direct bitable and wiki bitable; docx embedded resolver returns structured unsupported error if metadata is not discoverable

- [x] **Step 1: Write failing HTTP fixture tests**

Add an `httptest.Server` fixture for `/space/api/v1/bitable/<base>/clientvars` returning synthetic clientvars. Verify inspect calls the endpoint and parses metadata.

- [x] **Step 2: Run HTTP resolver tests and verify RED**

Run: `go test ./internal/bitable -run 'HTTP|Wiki|DocxEmbedded' -count=1`

Expected: FAIL because the HTTP client path is missing.

- [x] **Step 3: Implement HTTP client path**

Implement cookie loading, CSRF extraction, common headers, and `clientvars` GET using the same safe pattern as `docslocal`.

For docx embedded bitable, parse docx clientvars block_map for `view`/`file` candidates. If no base token is discoverable, return:

```text
docx embedded bitable resolver could not locate base token in supported view/file blocks
```

- [x] **Step 4: Run HTTP resolver tests and verify GREEN**

Run: `go test ./internal/bitable -run 'HTTP|Wiki|DocxEmbedded' -count=1`

Expected: PASS.

---

### Task 6: Docs, Routing, and Release Notes

**Files:**
- Modify: `README.md`
- Modify: `docs/agent-routing.md`
- Modify: `docs/go-python-parity.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION` only if preparing a release
- Modify: `skills/codex/using-ixf-toolbox/SKILL.md`
- Modify: `skills/claude-code/using-ixf-toolbox/SKILL.md`

**Interfaces:**
- Consumes: completed CLI behavior
- Produces: documented bitable routing and current capability boundary

- [x] **Step 1: Write documentation updates**

Document:

- `ixf bitable inspect --url <url> --json`
- `ixf bitable attach ... --dry-run --json`
- `ixf bitable attach --apply` fails closed until upload contract capture
- docx/wiki URLs are accepted as source locators, but writes target bitable data

- [x] **Step 2: Run docs/routing checks**

Run: `rg -n "bitable|ixf bitable|docs update|sheets update" README.md docs/agent-routing.md docs/go-python-parity.md skills`

Expected: routing says bitable attachment writes use `ixf bitable`, not docs or sheets.

---

### Task 7: Full Verification and Local Commit

**Files:**
- All files modified above

**Interfaces:**
- Consumes: all previous tasks
- Produces: verified local commit ready for push/release after user approval

- [x] **Step 1: Run focused tests**

Run:

```bash
go test ./internal/bitable ./cmd/ixf -count=1
```

Expected: PASS.

- [x] **Step 2: Run full tests and vet**

Run:

```bash
go test ./...
go vet ./...
git diff --check
```

Expected: all commands exit 0.

- [x] **Step 3: Commit locally**

Run:

```bash
git add .
git commit -m "feat: add bitable attachment dry-run"
```

Expected: one local implementation commit. Do not push until the user explicitly asks.

## Self-Review

- Spec coverage: Tasks cover command family, source resolution, dry-run planning, apply failure gate, privacy, docs, and verification.
- Placeholder scan: The plan contains no open placeholder markers or unspecified implementation steps; the only deferred behavior is the explicitly specified unsupported apply contract from the approved spec.
- Type consistency: `InspectConfig`, `AttachConfig`, `ParseClientVars`, `Inspect`, and `Attach` are defined before use in CLI tasks.
