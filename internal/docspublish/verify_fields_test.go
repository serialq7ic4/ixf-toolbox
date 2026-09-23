package docspublish

import "testing"

// The check this replaces was len(specs) == len(expectedRootIDs), which buildBlocks
// makes true by construction, so it could never fail and never read the document.
func TestExpectedRootsInOrderAcceptsWrittenOrder(t *testing.T) {
	rootChildren := []string{"a", "b", "c"}
	if !expectedRootsInOrder(rootChildren, []string{"a", "b", "c"}) {
		t.Fatal("blocks written in order should verify")
	}
}

// A patch insert passes only the inserted ids, so unrelated blocks around them must
// not cause a failure.
func TestExpectedRootsInOrderIgnoresUnrelatedBlocks(t *testing.T) {
	rootChildren := []string{"pre", "a", "middle", "b", "post"}
	if !expectedRootsInOrder(rootChildren, []string{"a", "b"}) {
		t.Fatal("expected ids interleaved with existing blocks should verify")
	}
}

// The property the tautology could not see: all blocks present but out of order.
func TestExpectedRootsInOrderRejectsScrambledOrder(t *testing.T) {
	rootChildren := []string{"c", "b", "a"}
	if expectedRootsInOrder(rootChildren, []string{"a", "b", "c"}) {
		t.Fatal("blocks present but reordered must not verify")
	}
}

func TestExpectedRootsInOrderRejectsMissingBlock(t *testing.T) {
	rootChildren := []string{"a", "c"}
	if expectedRootsInOrder(rootChildren, []string{"a", "b", "c"}) {
		t.Fatal("a missing block must not verify")
	}
}

func TestExpectedRootsInOrderAcceptsNoExpectations(t *testing.T) {
	if !expectedRootsInOrder([]string{"a"}, nil) {
		t.Fatal("no expected ids means nothing to check")
	}
}

// A duplicated id in the document should still satisfy the order, since the first
// match is enough to establish position.
func TestExpectedRootsInOrderToleratesRepeatedIDs(t *testing.T) {
	rootChildren := []string{"a", "a", "b"}
	if !expectedRootsInOrder(rootChildren, []string{"a", "b"}) {
		t.Fatal("a repeated id ahead of the sequence should not break ordering")
	}
}

func calloutBlockMap(children []string, extra map[string]any) map[string]any {
	blockMap := map[string]any{
		"callout_1": map[string]any{"data": map[string]any{
			"type":     "callout",
			"children": toAnySlice(children),
		}},
	}
	for id, block := range extra {
		blockMap[id] = block
	}
	return blockMap
}

func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func TestCalloutIsEmptyForACalloutWithNothingInIt(t *testing.T) {
	blockMap := calloutBlockMap(nil, nil)
	if !calloutIsEmpty(blockMap, "callout_1") {
		t.Fatal("a callout with no children is empty")
	}
}

func TestCalloutIsNotEmptyWithText(t *testing.T) {
	blockMap := calloutBlockMap([]string{"text_1"}, map[string]any{
		"text_1": map[string]any{"data": map[string]any{
			"type": "text", "text": attributedCLIText("content"),
		}},
	})
	if calloutIsEmpty(blockMap, "callout_1") {
		t.Fatal("a callout holding text is not empty")
	}
}

// The false-failure case: an image-only callout has no text, and was therefore
// counted as empty, failing writes that had not touched it.
func TestCalloutIsNotEmptyWithOnlyAnImage(t *testing.T) {
	blockMap := calloutBlockMap([]string{"image_1"}, map[string]any{
		"image_1": map[string]any{"data": map[string]any{
			"type":  "image",
			"image": map[string]any{"token": "boxr-token"},
		}},
	})
	if calloutIsEmpty(blockMap, "callout_1") {
		t.Fatal("a callout holding only an image must not count as empty")
	}
}

func TestCalloutIsNotEmptyWithOnlyATable(t *testing.T) {
	blockMap := calloutBlockMap([]string{"table_1"}, map[string]any{
		"table_1": map[string]any{"data": map[string]any{"type": "table"}},
	})
	if calloutIsEmpty(blockMap, "callout_1") {
		t.Fatal("a callout holding only a table must not count as empty")
	}
}

// Non-text content nested deeper than one level still counts as content.
func TestCalloutIsNotEmptyWithNestedImage(t *testing.T) {
	blockMap := calloutBlockMap([]string{"wrapper"}, map[string]any{
		"wrapper": map[string]any{"data": map[string]any{
			"type": "ordered", "children": []any{"image_1"},
		}},
		"image_1": map[string]any{"data": map[string]any{"type": "image"}},
	})
	if calloutIsEmpty(blockMap, "callout_1") {
		t.Fatal("a nested image is still content")
	}
}

// A blank text child leaves the callout genuinely empty.
func TestCalloutIsEmptyWithOnlyBlankText(t *testing.T) {
	blockMap := calloutBlockMap([]string{"text_1"}, map[string]any{
		"text_1": map[string]any{"data": map[string]any{
			"type": "text", "text": attributedCLIText("   "),
		}},
	})
	if !calloutIsEmpty(blockMap, "callout_1") {
		t.Fatal("a callout holding only whitespace is empty")
	}
}
