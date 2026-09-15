# Ordered List Child Blocks Implementation Plan

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

Goal: Fix Markdown ordered-list publishing so a fenced code block or Mermaid image immediately following an ordered item is stored as that item's child, preserving Feishu list numbering across publish, update, and patch operations while allowing reads to round-trip nested content.

Architecture: Keep Spec as the intermediate representation and add a generic Children []Spec tree. The parser creates children only for consecutive fenced-code blocks after an ordered item; one recursive builder flattens the tree into block entries while retaining parent-child IDs. The docx reader and post-write verifier consume the same tree relationship rules, so root-level list grouping, nested content, Mermaid upload, and patch safety do not have separate implementations.

Tech Stack: Go 1.24, existing internal/docspublish parser/publisher, existing internal/docx client-vars reader, existing internal/docxgraph, Go standard-library tests, external mmdc Mermaid renderer only.

## Global Constraints

- Keep the public CLI flags and the existing buildBlocks(specs, pageID, factory) ([]string, []blockEntry) test-facing signature unchanged.
- Add Children []Spec to docspublish.Spec; only ordered specs may contain parser-generated children in this change.
- Attach only fenced code or Mermaid image blocks that follow an ordered item through blank lines; ordinary paragraphs, headings, bullets, ordered items, tables, blockquotes, and unknown blocks terminate the child run.
- Treat 1., 1., 1. and 1., 2. as the same continuous ordered-list syntax; do not compare source numbers.
- Keep Mermaid rendering external and Go-only: prefer mmdc SVG and fall back to PNG through the existing image upload path; do not add Python or UI automation.
- Keep root children limited to top-level specs; every nested entry belongs in the same change map with parent_id and the parent's children IDs.
- Do not change the target wiki/document https://yf2ljykclb.xfchat.iflytek.com/wiki/TYqOw8M44iJScrky3j3rGJIzzWf during implementation or tests.
- Preserve existing publish/update/patch confirmation, complex-block, duplicate, idempotency, image upload, and outside-section safety behavior.
- This is a bug fix: release version is 3.27.4, changing only the final version component from 3.27.3.
- All GitHub network commands must set HTTPS_PROXY=http://127.0.0.1:7890 and HTTP_PROXY=http://127.0.0.1:7890.

---

### Task 1: Parse Ordered Child Blocks

Files:
- Modify: internal/docspublish/publish.go:43-50 to add Children []Spec.
- Modify: internal/docspublish/fragment.go:16-122 to parse indented fences and ordered child runs.
- Create: internal/docspublish/fragment_test.go for parser-only regression tests.

Interfaces:
- Consumes: existing markdownLines, parseInline, isMermaidFence, orderedPattern, and Spec fields.
- Produces: Spec.Children, fenceStart(line string) (info string, indent string, ok bool), parseFence(lines []string, index int) (Spec, int, bool), and parser output where an ordered item owns its immediately following fenced code or Mermaid image specs.

- [ ] Step 1: Write the failing parser tests

Create internal/docspublish/fragment_test.go with tests that assert the complete tree shape, not only flattened text:

~~~go
package docspublish

import (
	"strings"
	"testing"
)

func TestParseMarkdownFragmentAttachesConsecutiveFencesToOrderedItems(t *testing.T) {
	specs, err := ParseMarkdownFragment("1. First\n\n  ```Plain\n  command-one\n  ```\n\n  ```Plain\n  command-two\n  ```\n\n1. Second\n\n```bash\ncommand-three\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Kind != "ordered" || specs[1].Kind != "ordered" {
		t.Fatalf("specs = %#v, want two top-level ordered specs", specs)
	}
	if len(specs[0].Children) != 2 || specs[0].Children[0].Kind != "code" || specs[0].Children[1].Kind != "code" {
		t.Fatalf("first children = %#v, want two code children", specs[0].Children)
	}
	if specs[0].Children[0].Text != "command-one" || specs[0].Children[1].Text != "command-two" {
		t.Fatalf("first child text = %#v", specs[0].Children)
	}
	if len(specs[1].Children) != 1 || specs[1].Children[0].Text != "command-three" {
		t.Fatalf("second children = %#v, want command-three", specs[1].Children)
	}
}

func TestParseMarkdownFragmentAttachesMermaidFenceAsImageChild(t *testing.T) {
	specs, err := ParseMarkdownFragment("1. Diagram\n\n```Plain\nflowchart LR\n  A --> B\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || len(specs[0].Children) != 1 {
		t.Fatalf("specs = %#v, want one ordered child", specs)
	}
	child := specs[0].Children[0]
	if child.Kind != "image" || child.SourceKind != "mermaid" || !strings.Contains(child.Text, "flowchart LR") {
		t.Fatalf("mermaid child = %#v", child)
	}
}

func TestParseMarkdownFragmentStopsOrderedChildrenAtNonFence(t *testing.T) {
	specs, err := ParseMarkdownFragment("1. First\n\n```Plain\ncommand-one\n```\n\nBody stops the child run.\n\n```Plain\nroot-code\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 || len(specs[0].Children) != 1 || specs[1].Kind != "text" || specs[2].Kind != "code" {
		t.Fatalf("specs = %#v, want ordered, text, code", specs)
	}
}

func TestParseMarkdownFragmentAcceptsReaderIndentedFence(t *testing.T) {
	specs, err := ParseMarkdownFragment("1. Step\n\n  ```Plain\n  echo ready\n  ```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || len(specs[0].Children) != 1 || specs[0].Children[0].Text != "echo ready" {
		t.Fatalf("specs = %#v, want one de-indented code child", specs)
	}
}

func TestParseMarkdownFragmentDoesNotUseOrderedNumbersForGrouping(t *testing.T) {
	specs, err := ParseMarkdownFragment("1. First\n\n1. Second\n\n```Plain\nsecond-code\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Kind != "ordered" || specs[1].Kind != "ordered" || len(specs[1].Children) != 1 {
		t.Fatalf("specs = %#v, want two ordered specs with the second child", specs)
	}
}
~~~

In the actual test file, replace each ``` marker with three literal backticks in the Markdown fixture strings.

- [ ] Step 2: Run the parser tests and verify they fail

Run:

~~~bash
go test ./internal/docspublish -run 'TestParseMarkdownFragment(Attaches|Stops|AcceptsReader|DoesNotUse)' -count=1
~~~

Expected: FAIL because Spec has no Children field and the current parser emits flat code/image specs.

- [ ] Step 3: Add the tree field and fence helpers

Change the model to retain child order:

~~~go
type Spec struct {
	Kind       string
	SourceKind string
	Text       string
	Rows       [][]string
	Runs       []InlineRun
	RowRuns    [][][]InlineRun
	Children   []Spec
}
~~~

Implement fenceStart so it accepts zero through three leading ASCII spaces and a line beginning with three backticks, returning the exact indentation prefix and info string. Implement parseFence so it removes the opening fence indentation from each content line, accepts a closing fence with the same indentation, returns an image/mermaid spec when isMermaidFence(info, text) is true, and otherwise returns a code spec. Replace both top-level fence checks and paragraph stop checks with fenceStart.

After parsing an ordered item, scan past blank lines with consumeOrderedChildren. While the next non-empty line is a fence, call parseFence, append the returned spec to ordered.Children, advance to its returned index, and skip blank lines. Stop before every non-fence block, including another ordered item. Append the ordered parent once and never append its child specs to the top-level slice:

~~~go
if orderedPattern.MatchString(line) {
	ordered := specFromInline("ordered", parseInline(orderedPattern.ReplaceAllString(line, "")))
	index++
	ordered.Children, index = consumeOrderedChildren(lines, index)
	specs = append(specs, ordered)
	continue
}
~~~

The child scan must consume only fences and blank lines. A fence without a closing delimiter still becomes a code/image spec using the remaining lines and returns the end index, matching the current parser's permissive behavior.

- [ ] Step 4: Run the focused parser tests and existing Mermaid tests

Run:

~~~bash
go test ./internal/docspublish -run 'TestParseMarkdownFragment|TestParseMarkdownConvertsMermaidFencesToImageSpecs' -count=1
~~~

Expected: PASS, with existing top-level Mermaid behavior unchanged and new ordered children attached.

- [ ] Step 5: Commit the parser change

~~~bash
git add internal/docspublish/publish.go internal/docspublish/fragment.go internal/docspublish/fragment_test.go
git commit -m "fix: parse ordered list child blocks"
~~~

### Task 2: Centralize Recursive Spec Traversal

Files:
- Create: internal/docspublish/spec_tree.go for traversal, validation, and recursive counts.
- Create: internal/docspublish/spec_tree_test.go for tree-wide count and validation tests.
- Modify: internal/docspublish/publish.go:451-526 to use recursive helpers for summaries and dry-run metadata.
- Modify: internal/docspublish/patch.go:410-451 to include child text in fingerprints and required-text derivation.
- Modify: internal/docspublish/mermaid.go:97-99 to route renderer readiness through the centralized recursive count.

Interfaces:
- Consumes: Spec.Children from Task 1 and existing InlineRun, table row, Mermaid, and image fields.
- Produces: walkSpecs(specs []Spec, visit func(Spec)), countNestedSpecs(specs []Spec) int, validateSpecTree(specs []Spec) error, and recursive implementations of summarizeSpecs, countSpecsByKind, countSpecsBySourceKind, countBoldTextRuns, fingerprintSpecs, and patchVerifyRequiredText.

- [ ] Step 1: Write failing recursive traversal tests

Create tests using one ordered parent with a code child and a Mermaid image child:

~~~go
func TestSpecTreeCountsIncludeChildren(t *testing.T) {
	specs := []Spec{{Kind: "ordered", Text: "Step", Children: []Spec{
		{Kind: "code", Text: "echo one"},
		{Kind: "image", SourceKind: "mermaid", Text: "flowchart LR\nA-->B"},
	}}}
	counts := summarizeSpecs(specs)
	if counts["ordered"] != 1 || counts["code"] != 1 || counts["image"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
	if countSpecsBySourceKind(specs, "image", "mermaid") != 1 || countSpecsByKind(specs, "image") != 1 {
		t.Fatalf("recursive image counts are missing: %#v", counts)
	}
	if countNestedSpecs(specs) != 2 {
		t.Fatalf("nested count = %d, want 2", countNestedSpecs(specs))
	}
}

func TestSpecTreeRejectsChildrenOnUnsupportedParent(t *testing.T) {
	err := validateSpecTree([]Spec{{Kind: "text", Text: "body", Children: []Spec{{Kind: "code", Text: "bad"}}}})
	if err == nil || !strings.Contains(err.Error(), "spec kind \"text\" cannot contain children") {
		t.Fatalf("err = %v, want unsupported parent error", err)
	}
}

func TestSpecTreeFingerprintAndRequiredTextIncludeCodeChildren(t *testing.T) {
	specs := []Spec{{Kind: "ordered", Text: "Step", Children: []Spec{{Kind: "code", Text: "unique-command"}}}}
	if fingerprintSpecs(specs) == fingerprintSpecs([]Spec{{Kind: "ordered", Text: "Step"}}) {
		t.Fatal("fingerprint ignored child code text")
	}
	required := patchVerifyRequiredText(specs, nil)
	if !strings.Contains(strings.Join(required, "\n"), "unique-command") {
		t.Fatalf("required text = %#v, want child code text", required)
	}
}
~~~

- [ ] Step 2: Run the traversal tests and verify they fail

Run:

~~~bash
go test ./internal/docspublish -run 'TestSpecTree' -count=1
~~~

Expected: FAIL because summaries, image counts, fingerprints, and required-text derivation currently inspect only top-level specs.

- [ ] Step 3: Implement one recursive traversal and route all counters through it

Create internal/docspublish/spec_tree.go with this traversal contract:

~~~go
func walkSpecs(specs []Spec, visit func(Spec)) {
	for _, spec := range specs {
		visit(spec)
		walkSpecs(spec.Children, visit)
	}
}

func countNestedSpecs(specs []Spec) int {
	count := 0
	walkSpecs(specs, func(spec Spec) { count += len(spec.Children) })
	return count
}
~~~

Use walkSpecs in summarizeSpecs, countSpecsByKind, countSpecsBySourceKind, and countBoldTextRuns; table row runs remain counted when visiting their table spec. Use a recursive pre-order text flattener for fingerprintSpecs, preserving existing separators and including code text while retaining the existing rule that image specs contribute no required text. Make patchVerifyRequiredText recurse into children and continue skipping image specs while including table cells and code text.

Implement validateSpecTree to recurse through all specs, allow children only on ordered, and return an error containing the parent kind when another kind has non-empty children. Call this validation before any write path builds blocks; parsed Markdown already satisfies the invariant, while hand-built specs receive an explicit error rather than being silently flattened.

- [ ] Step 4: Run focused and existing metadata tests

Run:

~~~bash
go test ./internal/docspublish -run 'TestSpecTree|TestPublishMarkdownDryRun|TestParseMarkdownConvertsMermaid' -count=1
~~~

Expected: PASS, including recursive mermaidImageCount, plannedImageCount, bold counts, table counts, and existing dry-run output.

- [ ] Step 5: Commit recursive traversal

~~~bash
git add internal/docspublish/spec_tree.go internal/docspublish/spec_tree_test.go internal/docspublish/publish.go internal/docspublish/patch.go internal/docspublish/mermaid.go
git commit -m "fix: traverse nested document specs"
~~~

### Task 3: Build a Recursive Docx Block Tree

Files:
- Modify: internal/docspublish/publish.go:1222-1260 to replace the flat loop with a recursive builder while preserving the buildBlocks return signature.
- Modify: internal/docspublish/publish.go:95-276 and internal/docspublish/patch.go:80-307 to validate specs before writes and pass returned top-level IDs to later verification.
- Modify: internal/docspublish/publish_test.go with builder and change-map regression tests.
- Modify: internal/docspublish/patch_test.go with nested patch-map assertions.

Interfaces:
- Consumes: validated Spec trees, existing blockFactory specialized builders, and existing imageSource/blockEntry upload metadata.
- Produces: buildSpecBlock(spec Spec, parentID string, factory *blockFactory, imageOrdinal *int) (string, []blockEntry), unchanged buildBlocks(...) ([]string, []blockEntry), root-only top IDs, and flat entries containing every nested block.

- [ ] Step 1: Write failing builder and change-map tests

Add a test that parses two ordered items with code and Mermaid children, then asserts root and nested relationships. Add encoding/json to the test imports:

~~~go
func TestBuildBlocksNestsOrderedChildrenAndKeepsRootIDsFlat(t *testing.T) {
	_, specs, err := ParseMarkdown("# Title\n\n1. First\n\n```Plain\ncommand-one\n```\n\n1. Second\n\n```mermaid\nflowchart LR\n  A --> B\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	topIDs, entries := buildBlocks(specs, "page_1", newBlockFactory("author_fixture"))
	if len(topIDs) != 2 || len(entries) != 4 {
		t.Fatalf("topIDs=%#v entries=%#v, want two parents and four entries", topIDs, entries)
	}
	byID := map[string]map[string]any{}
	for _, entry := range entries {
		byID[entry.ID] = entry.Data
	}
	for index, topID := range topIDs {
		parent := byID[topID]
		if parent["type"] != "ordered" || parent["parent_id"] != "page_1" {
			t.Fatalf("top[%d] = %#v", index, parent)
		}
		children := asSlice(parent["children"])
		if len(children) != 1 {
			t.Fatalf("top[%d] children = %#v", index, children)
		}
		child := byID[asString(children[0])]
		if child["parent_id"] != topID {
			t.Fatalf("child parent = %#v, want %q", child, topID)
		}
	}
	if byID[asString(asSlice(byID[topIDs[1]]["children"])[0])]["type"] != "image" {
		t.Fatalf("second child = %#v, want image", byID[asString(asSlice(byID[topIDs[1]]["children"])[0])])
	}
}

func TestBuildPatchInsertChangeMapLinksOnlyTopLevelIDsAtRoot(t *testing.T) {
	specs, err := ParseMarkdownFragment("1. First\n\n```Plain\ncommand-one\n```")
	if err != nil {
		t.Fatal(err)
	}
	topIDs, entries := buildBlocks(specs, "page_1", newBlockFactory("author_fixture"))
	changeMap := buildPatchInsertChangeMap("page_1", map[string]any{"version": 7}, 2, topIDs, entries)
	raw, _ := json.Marshal(changeMap)
	serialized := string(raw)
	if !strings.Contains(serialized, "\"li\":\""+topIDs[0]+"\"") {
		t.Fatalf("root insert missing top id: %s", serialized)
	}
	if strings.Count(serialized, "\"li\":\"") != 1 {
		t.Fatalf("nested child was inserted at root: %s", serialized)
	}
	if !strings.Contains(serialized, "\"parent_id\":\""+topIDs[0]+"\"") {
		t.Fatalf("nested child parent missing: %s", serialized)
	}
}
~~~

- [ ] Step 2: Run builder tests and verify the relationship assertions fail

Run:

~~~bash
go test ./internal/docspublish -run 'TestBuild(BlocksNests|PatchInsertChangeMap)' -count=1
~~~

Expected: FAIL because the current builder creates four root entries and every block has the page as parent_id.

- [ ] Step 3: Implement the recursive builder with a document-wide image ordinal

Implement buildSpecBlock with the exact interface above. The implementation must:

1. Create the current block using the existing specialized factory for quote, callout, and table; use baseBlockWithRuns for ordinary kinds.
2. For image, increment the shared ordinal with *imageOrdinal = *imageOrdinal + 1, create factory.imageBlock(parentID), and retain imageSource{Kind, Text, Ordinal} in the entry.
3. Append the current block's entries before recursively appending child entries.
4. Allocate child IDs in spec order, call buildSpecBlock(childSpec, blockID, factory, imageOrdinal), and replace the current block's children value with those child IDs.
5. Return the current block ID and a flat entry list.

Use a helper such as:

~~~go
func setEntryChildren(entries []blockEntry, blockID string, childIDs []any) {
	for index := range entries {
		if entries[index].ID == blockID {
			entries[index].Data["children"] = childIDs
			return
		}
	}
}
~~~

The actual buildBlocks loop owns one imageOrdinal := 0, calls buildSpecBlock for every root spec, appends only the returned root ID to topIDs, and appends all returned entries to the shared flat slice. The builder must not append child IDs to page-level topIDs. Specialized quote/callout/table entries retain their existing internal child relationships; parser-generated children are permitted only on ordered specs by validateSpecTree.

Call validateSpecTree(specs) before every production build in publish, update, patch insert, and section replacement. Keep the builder signature unchanged because existing unit tests and callers use it.

- [ ] Step 4: Run block, image, and patch tests

Run:

~~~bash
go test ./internal/docspublish -run 'TestBuild|TestPatch|TestPublishMarkdown|TestMermaid' -count=1
~~~

Expected: PASS, including existing placeholder/upload expectations and new parent-child assertions. The new image child must still carry Image.Kind == "mermaid" and the next document-wide ordinal.

- [ ] Step 5: Commit recursive block construction

~~~bash
git add internal/docspublish/publish.go internal/docspublish/patch.go internal/docspublish/publish_test.go internal/docspublish/patch_test.go
git commit -m "fix: build ordered list child blocks"
~~~

### Task 4: Read Nested Lists and Reset Numbering at Sibling Boundaries

Files:
- Modify: internal/docx/docx.go:66-279 to render the root through renderChildren, reset ordered counters after non-ordered siblings, and recursively render ordered children.
- Modify: internal/docx/docx_test.go with nested code/image and list-boundary tests.

Interfaces:
- Consumes: blockTree relationships and existing renderBlock, renderChildren, renderImage, and code-language normalization.
- Produces: reader output with adjacent ordered siblings numbered continuously, non-ordered siblings resetting the same-parent sequence, and ordered child blocks rendered at two-space indentation.

- [ ] Step 1: Write failing reader tests

Add fixtures for adjacent ordered siblings, a historical same-level code interruption, and an ordered item with nested code and image children:

~~~go
func TestConvertClientVarsResetsOrderedNumbersAfterNonOrderedSibling(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{"type": "page", "children": []any{"ordered_1", "ordered_2", "code_1", "ordered_3"}}),
			"ordered_1": blockData(map[string]any{"type": "ordered", "parent_id": "page_1", "text": attributedText("First")}),
			"ordered_2": blockData(map[string]any{"type": "ordered", "parent_id": "page_1", "text": attributedText("Second")}),
			"code_1": blockData(map[string]any{"type": "code", "parent_id": "page_1", "text": attributedText("interrupt")}),
			"ordered_3": blockData(map[string]any{"type": "ordered", "parent_id": "page_1", "text": attributedText("Restart")}),
		},
	}
	result := ConvertClientVars(clientVars, "page_1")
	want := "1. First\n\n2. Second\n\n```\ninterrupt\n```\n\n1. Restart\n"
	if result.Markdown != want {
		t.Fatalf("markdown = %q, want %q", result.Markdown, want)
	}
}

func TestConvertClientVarsRendersOrderedChildCodeAndImage(t *testing.T) {
	clientVars := map[string]any{
		"block_map": map[string]any{
			"page_1": blockData(map[string]any{"type": "page", "children": []any{"ordered_1"}}),
			"ordered_1": blockData(map[string]any{"type": "ordered", "parent_id": "page_1", "children": []any{"code_1", "image_1"}, "text": attributedText("Deploy")}),
			"code_1": blockData(map[string]any{"type": "code", "parent_id": "ordered_1", "language": "Plain Text", "text": attributedText("kubectl apply\n--record")}),
			"image_1": blockData(map[string]any{"type": "image", "parent_id": "ordered_1", "image": map[string]any{"token": "image-token", "name": "diagram.svg", "mimeType": "image/svg+xml"}}),
		},
	}
	result := ConvertClientVarsWithOptions(clientVars, "page_1", Options{
		ResolveImage: func(reference ImageReference) ImageResolution {
			if reference.BlockID != "image_1" {
				t.Fatalf("image reference = %#v", reference)
			}
			return ImageResolution{MarkdownPath: "assets/diagram.svg", AltText: "diagram"}
		},
	})
	want := "1. Deploy\n\n  ```Plain\n  kubectl apply\n  --record\n  ```\n\n  ![diagram](assets/diagram.svg)\n"
	if result.Markdown != want {
		t.Fatalf("markdown = %q, want %q", result.Markdown, want)
	}
}
~~~

- [ ] Step 2: Run the reader tests and verify the old behavior is exposed

Run:

~~~bash
go test ./internal/docx -run 'TestConvertClientVars(ResetsOrdered|RendersOrderedChild)' -count=1
~~~

Expected: FAIL because ordered counters currently accumulate across a code sibling and the ordered renderer ignores its children.

- [ ] Step 3: Render children through one sibling-aware path

Change ConvertClientVarsWithOptions to render the page title and then call renderChildren on the root instead of iterating tree.order with a shared flat loop. In renderChildren, track whether the previous child of the current parent was ordered. Before rendering a non-ordered child, delete the counter for the current parent; an ordered child following a non-ordered child therefore starts at one. Keep counters keyed by the actual parent ID, so different parents never share numbering.

Change the ordered branch to render its line and then its children:

~~~go
case block.kind == "ordered":
	parentKey := block.parentID
	if parentKey == "" {
		parentKey = "__root__"
	}
	orderedCounters[parentKey]++
	line := strings.TrimRight(fmt.Sprintf("%s%d. %s", strings.Repeat("  ", depth), orderedCounters[parentKey], block.text), " ")
	children := renderChildren(tree, block, depth+1, seen, assets, warnings, options, orderedCounters)
	return joinNonEmpty("\n\n", line, indentMarkdownBlock(children, "  "))
~~~

Add indentMarkdownBlock(value, prefix string) string that prefixes every non-empty line with the prefix and leaves blank lines unchanged. Apply it only to children rendered under an ordered item; code fences, Mermaid image Markdown, and future child blocks remain visibly nested. Keep ordinary top-level code and existing bullet/callout/quote indentation unchanged.

- [ ] Step 4: Run all docx reader tests

Run:

~~~bash
go test ./internal/docx -count=1
~~~

Expected: PASS, including existing basic Markdown, rich-text, table, image, bullet, quote, callout, and unknown-block tests, plus the new ordered-list tests.

- [ ] Step 5: Commit reader behavior

~~~bash
git add internal/docx/docx.go internal/docx/docx_test.go
git commit -m "fix: round-trip nested ordered list blocks"
~~~

### Task 5: Verify Nested Relationships Across Publish, Update, and Patch

Files:
- Modify: internal/docspublish/publish.go:744-846 to add expected-root-aware nested verification while retaining the existing verifyMarkdownOutput wrapper.
- Modify: internal/docspublish/publish.go:170-191, internal/docspublish/publish.go:250-282, and internal/docspublish/patch.go:122-307 to pass generated top IDs to verification.
- Create: internal/docspublish/nested_verify_test.go for relationship and image-token verification tests.
- Modify: internal/docspublish/publish_test.go, internal/docspublish/patch_test.go, and existing stateful fixtures to cover apply responses containing nested block maps.

Interfaces:
- Consumes: flat blockEntry entries, expected top-level IDs returned by buildBlocks, docxgraph.Graph, and client-vars block_map data.
- Produces: verifyMarkdownOutputWithRoots(pageID string, referer string, requiredText []string, specs []Spec, expectedRootIDs []string) (map[string]any, error), verifyExpectedSpecTree(blockMap map[string]any, rootID string, spec Spec) (matchedNested int, missingNested int, missingImageTokens int, valid bool), and verification fields expectedNestedBlockCount, nestedBlockCount, missingNestedBlockCount, and missingImageTokenCount.

- [ ] Step 1: Write failing nested verification tests

Add a passing fixture with an ordered parent, a code child, and an image child with a bound token; add failing cases for a wrong parent_id, a missing child ID in the parent's children, and an image child without an image.token:

~~~go
func TestVerifyMarkdownOutputChecksOrderedChildEdgesAndImageToken(t *testing.T) {
	state := map[string]any{
		"doxrzPage": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type": "page",
				"children": []any{"ordered_1"},
			},
		},
		"ordered_1": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type": "ordered",
				"parent_id": "doxrzPage",
				"children": []any{"code_1", "image_1"},
				"text": attributedCLIText("Deploy"),
			},
		},
		"code_1": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type": "code",
				"parent_id": "ordered_1",
				"text": attributedCLIText("command"),
			},
		},
		"image_1": map[string]any{
			"version": 1,
			"data": map[string]any{
				"type": "image",
				"parent_id": "ordered_1",
				"image": map[string]any{"token": "bound-token"},
			},
		},
	}
	session, closeServer := newVerifyFixtureSession(t, state)
	defer closeServer()
	verify, err := session.verifyMarkdownOutputWithRoots(
		"doxrzPage",
		session.spaceAPI+"/docx/doxrzPage",
		[]string{"Deploy", "command"},
		[]Spec{{Kind: "ordered", Text: "Deploy", Children: []Spec{
			{Kind: "code", Text: "command"},
			{Kind: "image", SourceKind: "mermaid", Text: "flowchart LR\nA-->B"},
		}}},
		[]string{"ordered_1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if verify["ok"] != true || verify["expectedNestedBlockCount"] != 2 || verify["nestedBlockCount"] != 2 ||
		verify["missingNestedBlockCount"] != 0 || verify["missingImageTokenCount"] != 0 {
		t.Fatalf("verify = %#v", verify)
	}
}
~~~

The fixture helper may require wrapping state as map[string]any{"block_map": state}; use the existing helper's actual input contract rather than changing production request semantics. Add table-driven failing cases that mutate only one relationship/token field and assert verify["ok"] == false.

- [ ] Step 2: Run the verification tests and verify they fail

Run:

~~~bash
go test ./internal/docspublish -run 'TestVerifyMarkdownOutputChecksOrderedChildEdgesAndImageToken' -count=1
~~~

Expected: FAIL because the current verifier has no expected-root API and does not inspect nested parent-child edges or image tokens.

- [ ] Step 3: Implement expected-root-aware verification without breaking existing tests

Keep the existing method as a compatibility wrapper:

~~~go
func (session *publishSession) verifyMarkdownOutput(pageID string, referer string, requiredText []string, specs []Spec) (map[string]any, error) {
	return session.verifyMarkdownOutputWithRoots(pageID, referer, requiredText, specs, nil)
}
~~~

Retain the current retry loop and counts in verifyMarkdownOutputWithRoots. When expectedRootIDs is non-empty, verify each expected root ID exists in the remote root children and recursively compare each expected child with the corresponding parent children entry. Require child existence, expected type, expected parent_id, and for image specs a non-empty image.token. Count each matched expected child edge in nestedBlockCount; compute missingNestedBlockCount from the expected child-edge count; count missing image tokens separately. Set ok false for a missing root, missing edge, wrong parent, wrong kind, or missing image token.

Add these fields to every result map:

~~~go
"expectedNestedBlockCount": countNestedSpecs(specs),
"nestedBlockCount":         0,
"missingNestedBlockCount":   0,
"missingImageTokenCount":    0,
~~~

If no expected roots are supplied, keep nested verification disabled and leave the missing count at zero so existing direct verifier tests remain compatible. The apply call sites always pass the generated top IDs, so publish/update/patch writes receive the stronger check.

Pass topIDs from ApplyMarkdown, applyUpdateMarkdown, PatchInsertMarkdown, and PatchSectionMarkdown. For delete-only patches pass no roots and preserve deletion verification. Keep plannedTopLevelBlocks based on top IDs and plannedBlockEntries based on the flat entry slice.

Update patch fingerprint and implicit required text to use Task 2's recursive helpers. This prevents a second insert whose only difference is nested code from being treated as a duplicate and ensures nested code is included in post-write required-text checks.

- [ ] Step 4: Extend publish/update/patch fixtures and run focused tests

Run:

~~~bash
go test ./internal/docspublish -run 'TestVerify|TestBuild|TestPatch|TestPublish|TestUpdate' -count=1
~~~

Expected: PASS. The fixture server must return a block_map whose ordered parent contains the generated child IDs and whose generated image block has a non-empty token after the simulated binding response. Existing tests for quote, bold, image count, unchanged blocks, and outside-section preservation must remain green.

- [ ] Step 5: Run the CLI integration suite

Run:

~~~bash
go test ./cmd/ixf -run 'TestCLI(Docs|Bitable|Sheets|OKR|UpdateSelf)' -count=1
~~~

Expected: PASS, with existing dry-run/apply safety fields unchanged and recursive plannedBlockEntries/verification fields present for nested document fixtures.

- [ ] Step 6: Commit nested verification and call-site wiring

~~~bash
git add internal/docspublish/publish.go internal/docspublish/patch.go internal/docspublish/nested_verify_test.go internal/docspublish/publish_test.go internal/docspublish/patch_test.go cmd/ixf
git commit -m "fix: verify nested docx block relationships"
~~~

### Task 6: Release 3.27.4 and Validate the Generated Runtime Packages

Files:
- Modify: VERSION from 3.27.3 to 3.27.4.
- Modify: CHANGELOG.md with a dated 3.27.4 - 2026-09-14 section above 3.27.3.
- Regenerate: .agents/plugins/marketplace.json, .claude-plugin/marketplace.json, plugins/codex/ixf-toolbox, and plugins/claude/ixf-toolbox through go run ./cmd/pluginpack.
- Verify: README.md, docs/docs-update.md, docs/release.md, and generated skill files contain no stale version or obsolete flat-list guidance.

Interfaces:
- Consumes: all implementation commits from Tasks 1-5 and the repository's Go-only release workflow.
- Produces: a clean 3.27.4 tree, fresh Codex/Claude plugin packages, passing Go/release smoke checks, commit fix: release ordered list child block fix, tag v3.27.4, and GitHub main/tag pushes through the required proxy.

- [ ] Step 1: Add the release note and version

Insert this non-empty changelog section above 3.27.3:

~~~markdown
## 3.27.4 - 2026-09-14

- Fixed ordered Markdown list publishing so immediately following fenced code and Mermaid image blocks become children of their list item instead of interrupting Feishu ordered-list numbering.
- Fixed docx reads to reset ordered numbering after non-ordered siblings and to round-trip nested code/image children with stable indentation.
- Added recursive block-tree verification for nested parent-child edges and bound image tokens across publish, update, and patch writes.
~~~

Write exactly 3.27.4 to VERSION, then run git diff --check.

- [ ] Step 2: Regenerate and check native plugin packages

Run:

~~~bash
go run ./cmd/pluginpack
go run ./cmd/pluginpack --check
~~~

Expected: generation succeeds, the check exits 0, and both Codex and Claude generated runtime metadata report 3.27.4.

- [ ] Step 3: Run the complete Go verification matrix

Run:

~~~bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
scripts/smoke-native-plugins.sh
/tmp/ixf-go deps install --dry-run --json
~~~

Expected: every command exits 0; the binary reports 3.27.4; native plugin smoke checks report matching generated packages; no Python command is required.

- [ ] Step 4: Inspect the release diff and commit metadata

Run:

~~~bash
git diff --check
git status --short
git diff --stat HEAD~1..HEAD
~~~

Confirm that only the ordered-list implementation, tests, release metadata, and generated plugin outputs are present. Confirm the issue target wiki was not accessed by the test commands. Commit the release metadata and generated package update:

~~~bash
git add VERSION CHANGELOG.md .agents/plugins/marketplace.json .claude-plugin/marketplace.json plugins/codex/ixf-toolbox plugins/claude/ixf-toolbox
git commit -m "fix: release ordered list child block fix"
~~~

- [ ] Step 5: Push main and the release tag through the GitHub proxy

Run:

~~~bash
HTTPS_PROXY=http://127.0.0.1:7890 HTTP_PROXY=http://127.0.0.1:7890 git push origin main
git tag v3.27.4
HTTPS_PROXY=http://127.0.0.1:7890 HTTP_PROXY=http://127.0.0.1:7890 git push origin v3.27.4
~~~

Expected: origin/main contains all implementation and release commits, tag v3.27.4 points at the versioned commit, and the release workflow starts. Do not claim release completion until the workflow's Go tests, plugin check, artifact build, and smoke steps have passed.

## Plan Self-Review

- Spec coverage: parser grouping and indentation are in Task 1; recursive counts/fingerprints/readiness are in Task 2; parent-aware block construction is in Task 3; reader numbering and nested rendering are in Task 4; apply verification and all write paths are in Task 5; 3.27.4 metadata, generated packages, and proxy release operations are in Task 6.
- Scope coverage: no new CLI flags, no Python, no UI automation, no Mermaid renderer change, no target wiki mutation, and no change to existing complex-block or patch safety contracts.
- Type consistency: Spec.Children, buildSpecBlock, buildBlocks, verifyMarkdownOutputWithRoots, countNestedSpecs, and validateSpecTree are named consistently across tasks; buildBlocks keeps its existing two-result signature.
- Placeholder scan: every task names concrete files, commands, expected outcomes, test names, and commit messages; no deferred implementation item is required.
