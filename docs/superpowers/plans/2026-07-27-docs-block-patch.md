# Docs Block Patch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship API-only block-level docs patching so agents can insert Markdown/table content under a heading without replacing the whole document body.

**Architecture:** Keep `ixf docs update` as the existing destructive `replace_body` command. Add a separate `ixf docs patch` command family that reads the remote docx block graph, locates a heading/section, and submits only bounded `user_change` operations. Implement insert first, then add bounded section replace/delete after insert is stable.

**Tech Stack:** Go 1.24+, standard library HTTP/JSON, existing `cmd/ixf`, existing `internal/docspublish` authenticated session and block factory, new focused `internal/docxgraph` package, mocked `httptest` CLI integration tests.

## Global Constraints

- Go `ixf` only; do not call `ixfdoc`, `ixfwrite`, Python fallback readers, or Python-compatible writers.
- API-only for document patching; do not use Playwright for docs patch.
- All remote writes require explicit `--apply`; dry-run must not mutate remote state.
- `ixf docs update` remains `mode:replace_body` and `destructive:true`; do not silently change existing behavior.
- Localized insert must not submit `ld` or `od` operations for existing blocks.
- Do not commit cookies, CSRF tokens, private URLs, full document IDs, private response payloads, or generated private artifacts.
- Use placeholder tenant URLs in docs and tests.
- Each release stage must update `VERSION`, `CHANGELOG.md`, relevant docs/skills, commit, tag, push, and publish a GitHub Release.
- Main branch development is acceptable for this release train unless the user requests a worktree/branch.

---

## File Structure

- Create `internal/docxgraph/graph.go`: block graph types, client-vars parsing, heading extraction, root child ordering.
- Create `internal/docxgraph/locator.go`: heading normalization, heading match, section range, insert index calculation.
- Create `internal/docxgraph/signature.go`: old-block signature, section fingerprint, duplicate-fragment detection.
- Create `internal/docxgraph/graph_test.go`: graph construction and heading hierarchy tests.
- Create `internal/docxgraph/locator_test.go`: section range and insert index tests.
- Create `internal/docxgraph/signature_test.go`: unchanged-block and duplicate detection tests.
- Create `internal/docspublish/fragment.go`: Markdown fragment parser that does not require a level-1 title.
- Create `internal/docspublish/patch.go`: `PatchInsertMarkdown`, dry-run/apply payloads, wiki/docx target resolution, insert change map, verification.
- Create `internal/docspublish/patch_test.go`: insert change-map tests and apply verification unit tests.
- Modify `internal/docspublish/publish.go`: extract or reuse minimal shared helpers without changing existing publish/update behavior.
- Modify `cmd/ixf/main.go`: add `ixf docs patch insert`, then later `replace-section` and `delete-section`.
- Modify `cmd/ixf/main_test.go`: command help/arg parsing tests.
- Modify `cmd/ixf/cli_integration_test.go`: mocked dry-run/apply flows.
- Modify `repository_contract_test.go`: installed skill and README contract checks.
- Modify `docs/agent-routing.md`, `README.md`, `README.en.md`, `docs/docs-update.md`, `skills/*/ixf-docs-writer/SKILL.md`, `skills/*/using-ixf-toolbox/SKILL.md`: route localized insert requests away from `docs update`.
- Modify `VERSION`, `CHANGELOG.md`: version/release metadata at each stage.

---

### Task 1: v3.16.0 Docx Graph and Locator Foundation

**Files:**
- Create: `internal/docxgraph/graph.go`
- Create: `internal/docxgraph/locator.go`
- Create: `internal/docxgraph/signature.go`
- Create: `internal/docxgraph/graph_test.go`
- Create: `internal/docxgraph/locator_test.go`
- Create: `internal/docxgraph/signature_test.go`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `clientVars map[string]any`, docx object token string.
- Produces:
  - `func Build(clientVars map[string]any, objToken string) (Graph, error)`
  - `func NormalizeHeading(text string) string`
  - `func (g Graph) FindHeadingByText(text string) (HeadingRef, error)`
  - `func (g Graph) SectionRange(anchor HeadingRef) SectionRange`
  - `func (g Graph) InsertIndex(anchor HeadingRef, position InsertPosition) (int, error)`
  - `func (g Graph) RootSignature() Signature`
  - `func (g Graph) SectionFingerprint(r SectionRange) string`

- [ ] **Step 1: Write failing graph construction test**

Add this test shape to `internal/docxgraph/graph_test.go`:

```go
func TestBuildGraphPreservesRootChildrenAndHeadings(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockEntry(2, map[string]any{
				"type": "page", "children": []any{"h1", "p1", "h2", "callout_1"},
			}),
			"h1": blockEntry(1, map[string]any{
				"type": "heading2", "parent_id": "page_1", "text": attributedText("1.1 账号全集群初始化"),
			}),
			"p1": blockEntry(1, map[string]any{
				"type": "text", "parent_id": "page_1", "text": attributedText("existing body"),
			}),
			"h2": blockEntry(1, map[string]any{
				"type": "heading2", "parent_id": "page_1", "text": attributedText("1.2 其他章节"),
			}),
			"callout_1": blockEntry(1, map[string]any{
				"type": "callout", "parent_id": "page_1", "children": []any{"callout_text"},
			}),
			"callout_text": blockEntry(1, map[string]any{
				"type": "text", "parent_id": "callout_1", "text": attributedText("rich block"),
			}),
		},
	}

	graph, err := Build(clientVars, "page_1")
	if err != nil {
		t.Fatal(err)
	}
	if got := graph.RootChildren; !reflect.DeepEqual(got, []string{"h1", "p1", "h2", "callout_1"}) {
		t.Fatalf("root children = %#v", got)
	}
	heading, err := graph.FindHeadingByText("1.1 账号全集群初始化")
	if err != nil {
		t.Fatal(err)
	}
	if heading.ID != "h1" || heading.Level != 2 || heading.Index != 0 {
		t.Fatalf("heading = %#v", heading)
	}
}
```

Run: `go test ./internal/docxgraph`

Expected: FAIL because package/types do not exist.

- [ ] **Step 2: Implement minimal graph types and parser**

Create `internal/docxgraph/graph.go` with these public types:

```go
package docxgraph

type Block struct {
	ID       string
	Kind     string
	ParentID string
	Text     string
	Children []string
	Version  int
	Raw      map[string]any
}

type Graph struct {
	RootID       string
	RootVersion  int
	Blocks       map[string]Block
	RootChildren []string
}

type HeadingRef struct {
	ID    string
	Text  string
	Level int
	Index int
}

type SectionRange struct {
	Start int
	End   int
	IDs   []string
}
```

Implement `Build` by following existing `internal/docx/docx.go` parsing logic, but keep IDs, versions, raw data, and root child order. Copy only tiny helper equivalents needed for `asMap`, `stringValue`, attributed text extraction, and children parsing; do not import `internal/docx` private types.

Run: `gofmt -w internal/docxgraph && go test ./internal/docxgraph`

Expected: PASS for graph construction test.

- [ ] **Step 3: Write failing locator tests**

Add to `internal/docxgraph/locator_test.go`:

```go
func TestSectionEndInsertIndexStopsBeforeNextSameOrHigherHeading(t *testing.T) {
	graph := graphFixtureWithRootChildren(
		blockFixture("h1", "heading2", "1.1 账号全集群初始化"),
		blockFixture("p1", "text", "body"),
		blockFixture("h1_1", "heading3", "nested"),
		blockFixture("p2", "text", "nested body"),
		blockFixture("h2", "heading2", "1.2 其他章节"),
	)
	heading, err := graph.FindHeadingByText("1.1 账号全集群初始化")
	if err != nil {
		t.Fatal(err)
	}
	index, err := graph.InsertIndex(heading, PositionSectionEnd)
	if err != nil {
		t.Fatal(err)
	}
	if index != 4 {
		t.Fatalf("insert index = %d, want 4", index)
	}
}

func TestAfterHeadingInsertIndex(t *testing.T) {
	graph := graphFixtureWithRootChildren(
		blockFixture("h1", "heading2", "1.1 账号全集群初始化"),
		blockFixture("p1", "text", "body"),
	)
	heading, err := graph.FindHeadingByText("1.1 账号全集群初始化")
	if err != nil {
		t.Fatal(err)
	}
	index, err := graph.InsertIndex(heading, PositionAfterHeading)
	if err != nil {
		t.Fatal(err)
	}
	if index != 1 {
		t.Fatalf("insert index = %d, want 1", index)
	}
}
```

Run: `go test ./internal/docxgraph`

Expected: FAIL because locator functions do not exist.

- [ ] **Step 4: Implement heading normalization and range/index logic**

Create `internal/docxgraph/locator.go`:

```go
type InsertPosition string

const (
	PositionSectionEnd  InsertPosition = "section-end"
	PositionAfterHeading InsertPosition = "after-heading"
)
```

Implement:

- `NormalizeHeading`: trim, collapse whitespace, convert full-width space to normal space.
- `FindHeadingByText`: exact normalized match, fail on zero or multiple matches.
- `SectionRange`: start at anchor index, end before next heading with level `<= anchor.Level`, or document end.
- `InsertIndex`: `after-heading` returns `anchor.Index + 1`; `section-end` returns `SectionRange(anchor).End`.

Run: `go test ./internal/docxgraph`

Expected: PASS.

- [ ] **Step 5: Write and implement signature/fingerprint tests**

Add to `internal/docxgraph/signature_test.go`:

```go
func TestRootSignatureIgnoresVersionsButCatchesContentChanges(t *testing.T) {
	before := graphFixtureWithTextVersion("p1", "same", 1)
	afterVersionOnly := graphFixtureWithTextVersion("p1", "same", 2)
	afterContent := graphFixtureWithTextVersion("p1", "changed", 2)

	if !before.RootSignature().Equal(afterVersionOnly.RootSignature()) {
		t.Fatal("signature changed for version-only update")
	}
	if before.RootSignature().Equal(afterContent.RootSignature()) {
		t.Fatal("signature did not change for content update")
	}
}

func TestSectionFingerprintFindsDuplicateTableText(t *testing.T) {
	graph := graphFixtureWithRootChildren(
		blockFixture("h1", "heading2", "Target"),
		blockFixture("table_1", "table", "Name\tValue\nAlpha\t42"),
		blockFixture("h2", "heading2", "Next"),
	)
	heading, err := graph.FindHeadingByText("Target")
	if err != nil {
		t.Fatal(err)
	}
	rangeValue := graph.SectionRange(heading)
	if !graph.SectionContainsFingerprint(rangeValue, FingerprintText("Name\tValue\nAlpha\t42")) {
		t.Fatal("duplicate table fingerprint was not detected")
	}
}
```

Implement `Signature`, `RootSignature`, `FingerprintText`, `SectionFingerprint`, and `SectionContainsFingerprint` in `signature.go`. Use normalized block kind/text/children and ignore `Version`.

Run:

```bash
go test ./internal/docxgraph
go test ./...
go vet ./...
git diff --check
```

Expected: all pass.

- [ ] **Step 6: Release v3.16.0**

Update `VERSION` to `3.16.0` and add changelog:

```markdown
## 3.16.0 - 2026-07-27

- Added internal docx block graph, heading locator, section range, and signature foundations for future non-destructive docs patching.
- Added fixture coverage for heading insertion indexes and duplicate-fragment detection without adding a remote mutation surface.
```

Run:

```bash
gofmt -w internal/docxgraph
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.16.0" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go 3.16.0
git diff --check
git add internal/docxgraph VERSION CHANGELOG.md
git commit -m "feat: add docx graph patch foundation"
git tag v3.16.0
HTTP_PROXY=http://127.0.0.1:7890 HTTPS_PROXY=http://127.0.0.1:7890 git push origin main v3.16.0
```

Expected: release workflow creates v3.16.0 artifacts.

---

### Task 2: v3.17.0 Patch Insert Dry-Run

**Files:**
- Create: `internal/docspublish/fragment.go`
- Create: `internal/docspublish/patch.go`
- Create: `internal/docspublish/patch_test.go`
- Modify: `internal/docspublish/publish.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes:
  - `docxgraph.Build`
  - existing `newPublishSession`, `clientVars`, `buildBlocks`, `withTableFallbackMetadata`
- Produces:
  - `type PatchInsertConfig struct { MarkdownPath, URL, CookiesPath, SpaceAPI, UnderHeading, Position string; RequiredText []string; Apply bool }`
  - `func PatchInsertMarkdown(config PatchInsertConfig) (map[string]any, error)`
  - `func ParseMarkdownFragment(markdown string) ([]Spec, error)`

- [ ] **Step 1: Write failing fragment parser tests**

Add to `internal/docspublish/patch_test.go`:

```go
func TestParseMarkdownFragmentAcceptsTableWithoutTitle(t *testing.T) {
	specs, err := ParseMarkdownFragment("| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Kind != "table" || specs[0].Rows[1][0] != "Alpha" {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestParseMarkdownFragmentDoesNotRenameDocumentForH1(t *testing.T) {
	specs, err := ParseMarkdownFragment("# Inserted Heading\n\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Kind != "heading1" || specs[0].Text != "Inserted Heading" {
		t.Fatalf("specs = %#v", specs)
	}
}
```

Run: `go test ./internal/docspublish -run Fragment`

Expected: FAIL because `ParseMarkdownFragment` does not exist.

- [ ] **Step 2: Implement Markdown fragment parser**

Create `internal/docspublish/fragment.go`. Extract the body parser from `ParseMarkdown` into:

```go
func ParseMarkdownFragment(markdown string) ([]Spec, error)
```

Then update `ParseMarkdown` to call the shared body parser after validating the first `# title` line. Do not change existing publish/update behavior.

Run:

```bash
gofmt -w internal/docspublish/fragment.go internal/docspublish/publish.go
go test ./internal/docspublish -run 'Fragment|ParseMarkdown'
```

Expected: PASS.

- [ ] **Step 3: Write failing dry-run unit tests**

Add to `internal/docspublish/patch_test.go`:

```go
func TestPatchInsertDryRunReportsNonDestructivePlan(t *testing.T) {
	server := patchInsertFixtureServer(t)
	markdownPath := writeTempMarkdown(t, "| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")

	payload, err := PatchInsertMarkdown(PatchInsertConfig{
		MarkdownPath:  markdownPath,
		URL:           server.URL + "/docx/page_1",
		SpaceAPI:      server.URL,
		CookiesPath:   writeCookieFixture(t),
		UnderHeading: "1.1 账号全集群初始化",
		Position:     "section-end",
		Apply:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["operation"] != "docs_patch_insert" || payload["mode"] != "block_insert" ||
		payload["destructive"] != false || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["tableBlockType"] != "table" || payload["tableFallbackCount"] != 0 {
		t.Fatalf("table metadata = %+v", payload)
	}
	if payload["duplicateCandidate"] != false || payload["existingBlocksTouched"] != false {
		t.Fatalf("safety metadata = %+v", payload)
	}
}
```

Run: `go test ./internal/docspublish -run PatchInsertDryRun`

Expected: FAIL because dry-run implementation does not exist.

- [ ] **Step 4: Implement patch insert dry-run**

Create `internal/docspublish/patch.go`:

- parse target URL as docx or wiki-backed docx;
- for docx, get token from `/docx/`;
- for wiki, fetch HTML and resolve backing docx token using existing token extraction behavior;
- read target `client_vars`;
- build `docxgraph.Graph`;
- find heading and insert index;
- parse fragment specs;
- build new blocks with `buildBlocks(specs, target.Token, newBlockFactory(memberID))` but do not write them;
- compute `insertFingerprint` and `duplicateCandidate`;
- return dry-run JSON.

Dry-run payload must include:

```go
map[string]any{
	"ok": true,
	"dryRun": true,
	"operation": "docs_patch_insert",
	"mode": "block_insert",
	"destructive": false,
	"willWrite": false,
	"targetKind": "docx",
	"anchorHeading": config.UnderHeading,
	"position": position,
	"insertIndex": insertIndex,
	"tableBlockType": "table",
	"tableFallbackCount": 0,
	"existingBlocksTouched": false,
}
```

Run: `go test ./internal/docspublish -run PatchInsertDryRun`

Expected: PASS.

- [ ] **Step 5: Add CLI parse/help tests**

Add tests to `cmd/ixf/main_test.go`:

```go
func TestDocsPatchInsertHelp(t *testing.T) {
	code, stdout, stderr := runCLIForTest("docs", "patch", "insert", "--help")
	if code != 0 || !strings.Contains(stdout, "ixf docs patch insert") || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestParseDocsPatchInsertArgs(t *testing.T) {
	parsed, err := parseDocsPatchInsertArgs([]string{
		"table.md", "--url", "https://tenant.example.test/wiki/example",
		"--under-heading", "1.1 账号全集群初始化", "--position", "section-end", "--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.markdown != "table.md" || parsed.position != "section-end" || !parsed.dryRun {
		t.Fatalf("parsed = %#v", parsed)
	}
}
```

Run: `go test ./cmd/ixf -run 'DocsPatch|ParseDocsPatch'`

Expected: FAIL.

- [ ] **Step 6: Implement CLI dry-run command**

Modify `cmd/ixf/main.go`:

- `runDocs` includes `patch`;
- `runDocsPatch` handles `insert`;
- `printDocsPatchInsertHelp` documents flags;
- `parseDocsPatchInsertArgs` supports `--url`, `--under-heading`, `--position`, `--require`, `--cookies`, `--space-api`, `--dry-run`, `--apply`;
- `runDocsPatchInsert` calls `docspublish.PatchInsertMarkdown`.

Run:

```bash
gofmt -w cmd/ixf/main.go
go test ./cmd/ixf -run 'DocsPatch|ParseDocsPatch'
```

Expected: PASS.

- [ ] **Step 7: Add mocked CLI dry-run integration test**

Add to `cmd/ixf/cli_integration_test.go`:

```go
func TestDocsPatchInsertDryRunCLI(t *testing.T) {
	server := newDocsPatchInsertServer(t)
	input := writeTempFile(t, "table.md", "| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")
	code, stdout, stderr := runCLIForTest(
		"docs", "patch", "insert", input,
		"--url", server.URL+"/docx/page_1",
		"--space-api", server.URL,
		"--cookies", writeCookieFixture(t),
		"--under-heading", "1.1 账号全集群初始化",
		"--dry-run",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["operation"] != "docs_patch_insert" || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
}
```

Run: `go test ./cmd/ixf -run DocsPatchInsertDryRunCLI`

Expected: PASS after fixture server is implemented.

- [ ] **Step 8: Release v3.17.0**

Update `VERSION` to `3.17.0` and changelog:

```markdown
## 3.17.0 - 2026-07-27

- Added `ixf docs patch insert --dry-run` for non-destructive docx/wiki-backed docx insertion planning.
- Added Markdown fragment parsing, heading locator metadata, duplicate-candidate detection, and native table dry-run metadata for block insert plans.
```

Run:

```bash
gofmt -w internal/docspublish cmd/ixf
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.17.0" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go 3.17.0
git diff --check
git add internal/docspublish cmd/ixf VERSION CHANGELOG.md
git commit -m "feat: add docs patch insert dry-run"
git tag v3.17.0
HTTP_PROXY=http://127.0.0.1:7890 HTTPS_PROXY=http://127.0.0.1:7890 git push origin main v3.17.0
```

Expected: release workflow creates v3.17.0 artifacts.

---

### Task 3: v3.18.0 Patch Insert Apply

**Files:**
- Modify: `internal/docspublish/patch.go`
- Modify: `internal/docspublish/patch_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `docs/release.md`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `PatchInsertMarkdown` dry-run planner and existing `publishSession.writeBlocks`.
- Produces:
  - apply path that submits new blocks and root `children` `li` ops only;
  - post-write verification with unchanged old block signatures;
  - duplicate insert refusal.

- [ ] **Step 1: Write failing change-map unit test**

Add to `internal/docspublish/patch_test.go`:

```go
func TestBuildPatchInsertChangeMapOnlyAddsNewBlocksAndRootLinks(t *testing.T) {
	root := map[string]any{"version": 7}
	topIDs := []string{"new_table", "new_text"}
	entries := []blockEntry{
		{ID: "new_table", Data: map[string]any{"type": "table", "parent_id": "page_1"}},
		{ID: "new_text", Data: map[string]any{"type": "text", "parent_id": "page_1"}},
	}
	changeMap := buildPatchInsertChangeMap("page_1", root, 2, topIDs, entries)
	raw, err := json.Marshal(changeMap)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{`"ld"`, `"od"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("insert change map touched existing blocks: %s", text)
		}
	}
	if !strings.Contains(text, `"li":"new_table"`) || !strings.Contains(text, `"li":"new_text"`) {
		t.Fatalf("missing root insert ops: %s", text)
	}
}
```

Run: `go test ./internal/docspublish -run PatchInsertChangeMap`

Expected: FAIL.

- [ ] **Step 2: Implement insert change map**

Add to `patch.go`:

```go
func buildPatchInsertChangeMap(pageID string, root map[string]any, insertIndex int, topIDs []string, entries []blockEntry) map[string]any
```

Implementation rules:

- root block payload has `ops: insertChildOpsAt(insertIndex, topIDs)`;
- each new block has `oi` object insert;
- no old child deletion;
- top-level order is preserved.

Run: `go test ./internal/docspublish -run PatchInsertChangeMap`

Expected: PASS.

- [ ] **Step 3: Write failing apply integration test**

Add to `cmd/ixf/cli_integration_test.go`:

```go
func TestDocsPatchInsertApplyPreservesExistingBlocks(t *testing.T) {
	server := newDocsPatchInsertApplyServer(t)
	input := writeTempFile(t, "table.md", "| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")
	code, stdout, stderr := runCLIForTest(
		"docs", "patch", "insert", input,
		"--url", server.URL+"/docx/page_1",
		"--space-api", server.URL,
		"--cookies", writeCookieFixture(t),
		"--under-heading", "1.1 账号全集群初始化",
		"--require", "Alpha",
		"--apply",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatal(err)
	}
	verify := payload["verify"].(map[string]any)
	if verify["ok"] != true || verify["unchangedExistingBlocks"] != true {
		t.Fatalf("verify = %+v", verify)
	}
	if payload["existingBlocksTouched"] != false {
		t.Fatalf("payload = %+v", payload)
	}
}
```

Run: `go test ./cmd/ixf -run DocsPatchInsertApply`

Expected: FAIL.

- [ ] **Step 4: Implement apply and verification**

In `PatchInsertMarkdown`:

- if `Apply=false`, keep dry-run behavior;
- if `Apply=true`, refetch state, recompute locator and duplicate candidate;
- refuse duplicate candidate before write;
- build change map and call `session.writeBlocks`;
- refetch and build new graph;
- verify old root signature unchanged except for inserted root children;
- verify old block subtree signatures unchanged;
- verify required text or derived fragment text exists;
- verify inserted block count and table metadata.

Apply payload includes:

```go
"dryRun": false,
"willWrite": true,
"existingBlocksTouched": false,
"insertedTopLevelBlocks": len(topIDs),
"insertedBlockCount": len(entries),
"verify": map[string]any{
	"ok": true,
	"unchangedExistingBlocks": true,
	"missingRequiredText": []string{},
}
```

Run:

```bash
gofmt -w internal/docspublish cmd/ixf
go test ./internal/docspublish -run PatchInsert
go test ./cmd/ixf -run DocsPatchInsertApply
```

Expected: PASS.

- [ ] **Step 5: Write duplicate apply refusal test**

Add to `cmd/ixf/cli_integration_test.go`:

```go
func TestDocsPatchInsertApplyRefusesDuplicate(t *testing.T) {
	server := newDocsPatchDuplicateServer(t)
	input := writeTempFile(t, "table.md", "| Name | Value |\n| --- | --- |\n| Alpha | 42 |\n")
	code, stdout, stderr := runCLIForTest(
		"docs", "patch", "insert", input,
		"--url", server.URL+"/docx/page_1",
		"--space-api", server.URL,
		"--cookies", writeCookieFixture(t),
		"--under-heading", "1.1 账号全集群初始化",
		"--apply",
	)
	if code != 2 || !strings.Contains(stderr, "duplicate insert candidate") {
		t.Fatalf("code=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if server.WriteCount() != 0 {
		t.Fatalf("duplicate apply wrote %d times", server.WriteCount())
	}
}
```

Implement duplicate refusal and rerun targeted tests.

- [ ] **Step 6: Manual live smoke with non-sensitive doc**

Use a non-sensitive wiki/docx created for smoke only. It must contain:

- target heading;
- real web-editor high-light/callout block;
- image or embedded sheet if available;
- table insertion target.

Run:

```bash
ixf docs patch insert /tmp/non-sensitive-table.md \
  --url 'https://tenant.example.test/wiki/example' \
  --under-heading '1.1 Account initialization' \
  --dry-run
```

After explicit approval, run:

```bash
ixf docs patch insert /tmp/non-sensitive-table.md \
  --url 'https://tenant.example.test/wiki/example' \
  --under-heading '1.1 Account initialization' \
  --require 'Alpha' \
  --apply
```

Read back:

```bash
ixf docs read 'https://tenant.example.test/wiki/example' --out-dir /tmp/ixf-patch-smoke --print-manifest
```

Expected: inserted table appears, existing high-light block remains visually intact, and duplicate second apply is refused.

- [ ] **Step 7: Release v3.18.0**

Update `VERSION` to `3.18.0` and changelog:

```markdown
## 3.18.0 - 2026-07-27

- Added `ixf docs patch insert --apply` for API-only block insert writes.
- Added unchanged-existing-block verification, duplicate insert refusal, and native table insertion apply coverage.
```

Run full verification and release:

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.18.0" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go 3.18.0
git diff --check
git add internal/docspublish cmd/ixf docs/release.md VERSION CHANGELOG.md
git commit -m "feat: apply docs patch insert"
git tag v3.18.0
HTTP_PROXY=http://127.0.0.1:7890 HTTPS_PROXY=http://127.0.0.1:7890 git push origin main v3.18.0
```

Expected: release workflow creates v3.18.0 artifacts.

---

### Task 4: v3.19.0 Agent Routing and Documentation

**Files:**
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/agent-routing.md`
- Modify: `docs/docs-update.md`
- Modify: `skills/codex/ixf-docs-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-docs-writer/SKILL.md`
- Modify: `skills/codex/using-ixf-toolbox/SKILL.md`
- Modify: `skills/claude-code/using-ixf-toolbox/SKILL.md`
- Modify: `repository_contract_test.go`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: working `ixf docs patch insert`.
- Produces: agent guidance that routes localized insert/append requests to `docs patch insert`, not `docs update`.

- [ ] **Step 1: Write failing repository contract tests**

Add checks to `repository_contract_test.go`:

```go
func TestDocsWriterSkillRoutesLocalizedInsertToPatch(t *testing.T) {
	for _, path := range []string{
		"skills/codex/ixf-docs-writer/SKILL.md",
		"skills/claude-code/ixf-docs-writer/SKILL.md",
	} {
		content := readFile(t, path)
		assertContains(t, content, "ixf docs patch insert")
		assertContains(t, content, "insert under heading")
		assertContains(t, content, "do not use `ixf docs update` for localized insertion")
	}
}
```

Run: `go test ./... -run DocsWriterSkillRoutesLocalizedInsertToPatch`

Expected: FAIL.

- [ ] **Step 2: Update skills and routing docs**

Document:

- `docs update` is only for whole-body replacement;
- localized insert/append goes to `ixf docs patch insert`;
- dry-run first, apply only after confirmation;
- inspect `duplicateCandidate`, `existingBlocksTouched`, and `verify.unchangedExistingBlocks`;
- if `duplicateCandidate:true`, stop instead of apply.

Run contract test again.

Expected: PASS.

- [ ] **Step 3: Update README command table and examples**

Add command rows:

```markdown
| `ixf docs patch insert <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --dry-run` | Plan a non-destructive block insertion under a heading |
| `ixf docs patch insert <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --apply` | Insert confirmed fragment blocks without replacing existing document body |
```

Add natural prompt examples:

```markdown
> 把这个表格插入到文档的 `1.1 账号全集群初始化` 章节下，先展示 dry-run 计划。
```

Run:

```bash
go test ./...
go vet ./...
git diff --check
```

Expected: PASS.

- [ ] **Step 4: Release v3.19.0**

Update `VERSION` to `3.19.0` and changelog:

```markdown
## 3.19.0 - 2026-07-27

- Updated README, routing docs, and installed skills so localized document insert requests use `ixf docs patch insert` instead of destructive `ixf docs update`.
- Added repository contract coverage for docs patch insert agent guidance.
```

Run release verification:

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.19.0" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go 3.19.0
git diff --check
git add README.md README.en.md docs skills repository_contract_test.go VERSION CHANGELOG.md
git commit -m "docs: route localized docs edits to patch insert"
git tag v3.19.0
HTTP_PROXY=http://127.0.0.1:7890 HTTPS_PROXY=http://127.0.0.1:7890 git push origin main v3.19.0
```

Expected: release workflow creates v3.19.0 artifacts and installed skills can be refreshed.

---

### Task 5: v3.20.0 Bounded Replace/Delete Section

**Files:**
- Modify: `internal/docspublish/patch.go`
- Modify: `internal/docspublish/patch_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/agent-routing.md`
- Modify: `skills/codex/ixf-docs-writer/SKILL.md`
- Modify: `skills/claude-code/ixf-docs-writer/SKILL.md`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: docxgraph section ranges and patch insert block generation.
- Produces:
  - `ixf docs patch replace-section <fragment.md> --url ... --under-heading ... --dry-run|--apply`
  - `ixf docs patch delete-section --url ... --under-heading ... --dry-run|--apply`
  - section-only destructive safeguards.

- [ ] **Step 1: Write failing bounded replace tests**

Add to `internal/docspublish/patch_test.go`:

```go
func TestReplaceSectionChangeMapTouchesOnlySectionChildren(t *testing.T) {
	graph := replaceSectionGraphFixture()
	heading, err := graph.FindHeadingByText("Replace Me")
	if err != nil {
		t.Fatal(err)
	}
	r := graph.SectionRange(heading)
	changeMap := buildReplaceSectionChangeMap("page_1", map[string]any{"version": 3}, r, []string{"new_1"}, []blockEntry{
		{ID: "new_1", Data: map[string]any{"type": "text", "parent_id": "page_1"}},
	})
	raw, _ := json.Marshal(changeMap)
	text := string(raw)
	if strings.Contains(text, "outside_before") || strings.Contains(text, "outside_after") {
		t.Fatalf("outside section was touched: %s", text)
	}
}
```

Run: `go test ./internal/docspublish -run ReplaceSection`

Expected: FAIL.

- [ ] **Step 2: Implement bounded section replacement**

Add:

```go
type PatchSectionConfig struct {
	MarkdownPath  string
	URL           string
	CookiesPath   string
	SpaceAPI      string
	UnderHeading  string
	RequiredText  []string
	AllowComplex  bool
	Apply         bool
	DeleteOnly    bool
}
```

Rules:

- section range includes the heading and its body until next same/higher heading;
- default rejects section range containing complex blocks;
- `--allow-complex-section-replace` is required to replace/delete complex section content;
- outside-section signatures must remain unchanged.

Run targeted tests.

- [ ] **Step 3: Add CLI replace-section and delete-section**

Modify CLI:

```bash
ixf docs patch replace-section section.md --url DOC_OR_WIKI_URL --under-heading HEADING [--dry-run|--apply]
ixf docs patch delete-section --url DOC_OR_WIKI_URL --under-heading HEADING [--dry-run|--apply]
```

Add help tests and parse tests for both commands.

Run: `go test ./cmd/ixf -run 'DocsPatch.*Section|ParseDocsPatch'`

Expected: PASS.

- [ ] **Step 4: Add integration tests**

Add mocked CLI tests:

- replace section dry-run reports `mode:"section_replace"` and `destructive:true`;
- replace section apply preserves outside-section high-light block;
- delete section dry-run reports deleted root children count;
- complex section rejects without override.

Run: `go test ./cmd/ixf -run 'DocsPatchReplaceSection|DocsPatchDeleteSection'`

Expected: PASS.

- [ ] **Step 5: Update docs and skills**

Document:

- insert remains preferred for additive edits;
- replace/delete section are bounded destructive operations;
- complex section replacement requires explicit override;
- agents must not use section replace/delete for simple insertion.

Run:

```bash
go test ./...
go vet ./...
git diff --check
```

Expected: PASS.

- [ ] **Step 6: Release v3.20.0**

Update `VERSION` to `3.20.0` and changelog:

```markdown
## 3.20.0 - 2026-07-27

- Added bounded `ixf docs patch replace-section` and `ixf docs patch delete-section` operations with section-only destructive safeguards.
- Added verification that outside-section blocks remain unchanged and complex section content requires an explicit override.
```

Run release verification:

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=3.20.0" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go 3.20.0
git diff --check
git add internal/docspublish cmd/ixf README.md README.en.md docs skills VERSION CHANGELOG.md
git commit -m "feat: add bounded docs section patching"
git tag v3.20.0
HTTP_PROXY=http://127.0.0.1:7890 HTTPS_PROXY=http://127.0.0.1:7890 git push origin main v3.20.0
```

Expected: release workflow creates v3.20.0 artifacts.

---

### Task 6: v3.21.0+ Evaluate Move/Block-ID Operations

**Files:**
- Create a new dated advanced block patch spec under `docs/superpowers/specs/`
  only after v3.18-v3.20 live usage shows a real need.
- Do not modify implementation files in this task.
- Future implementation files are intentionally deferred until the advanced spec
  is approved.

**Interfaces:**
- Consumes: live usage evidence from v3.18-v3.20.
- Produces: decision on whether to implement `move-section`, `replace-block`, and richer locators.

- [ ] **Step 1: Collect evidence from patch insert and section patch use**

Review live support notes and failures:

```bash
rg -n "docs patch|patch insert|replace-section|delete-section" docs CHANGELOG.md
```

Expected: enough examples to decide whether advanced operations solve real workflows.

- [ ] **Step 2: Decide with user before implementation**

Prepare a short recommendation:

- implement `move-section` if users need reorder without regenerating blocks;
- implement `replace-block` only if `docs structure` diagnostics are reliable enough;
- defer arbitrary semantic diff because it risks reintroducing Markdown lossy behavior.

Expected: no code work starts until a new spec is approved.

---

## Self-Review Checklist

- Spec requirement "separate from replace_body": covered by Tasks 2-4.
- Spec requirement "docx and wiki-backed docx": covered by Task 2 dry-run and Task 3 apply tests.
- Spec requirement "no old block ld/od during insert": covered by Task 3 change-map tests.
- Spec requirement "duplicate insert protection": covered by Tasks 1-3.
- Spec requirement "native table inserts": covered by Tasks 2-3.
- Spec requirement "agent routing": covered by Task 4.
- Spec requirement "future replace/delete": covered by Task 5.
- Out of scope remains out of scope: comments, permissions, bitable edits, sheets cells, visual diff UI, arbitrary semantic diff.
