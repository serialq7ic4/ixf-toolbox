package docspublish

import "testing"

// The issue #5 regression: every code block legitimately single-line. The old
// heuristic required a newline somewhere and failed these documents.
func TestMissingCodeBlockTextsAcceptsAllSingleLineBlocks(t *testing.T) {
	specs := []Spec{
		{Kind: "ordered", Text: "step one", Children: []Spec{{Kind: "code", Text: "cmd-1"}}},
		{Kind: "ordered", Text: "step two", Children: []Spec{{Kind: "code", Text: "cmd-2"}}},
	}
	missing := missingCodeBlockTexts(specs, []string{"cmd-1", "cmd-2"})
	if len(missing) != 0 {
		t.Fatalf("single-line code blocks should verify, got missing %v", missing)
	}
}

// What the old heuristic was reaching for: a multi-line block arriving flattened.
// Comparing against the source still catches it.
func TestMissingCodeBlockTextsDetectsFlattenedMultilineBlock(t *testing.T) {
	specs := []Spec{{Kind: "code", Text: "line one\nline two\nline three"}}
	missing := missingCodeBlockTexts(specs, []string{"line oneline twoline three"})
	if len(missing) != 1 {
		t.Fatalf("a flattened multi-line block should be reported, got %v", missing)
	}
}

// The old heuristic passed a document where one multi-line block masked every
// other block being lost. The replacement reports each missing block.
func TestMissingCodeBlockTextsDoesNotLetOneGoodBlockMaskOthers(t *testing.T) {
	specs := []Spec{
		{Kind: "code", Text: "alpha\nbeta"},
		{Kind: "code", Text: "gamma"},
		{Kind: "code", Text: "delta"},
	}
	missing := missingCodeBlockTexts(specs, []string{"alpha\nbeta"})
	if len(missing) != 2 {
		t.Fatalf("expected the two lost blocks to be reported, got %v", missing)
	}
}

func TestMissingCodeBlockTextsMatchesMultilineRoundTrip(t *testing.T) {
	specs := []Spec{{Kind: "code", Text: "line one\nline two"}}
	if missing := missingCodeBlockTexts(specs, []string{"line one\nline two"}); len(missing) != 0 {
		t.Fatalf("an intact multi-line block should verify, got %v", missing)
	}
}

// Identical code blocks are common; each occurrence must be accounted for
// separately rather than one recovered block satisfying several expectations.
func TestMissingCodeBlockTextsCountsDuplicatesIndependently(t *testing.T) {
	specs := []Spec{
		{Kind: "code", Text: "same"},
		{Kind: "code", Text: "same"},
	}
	if missing := missingCodeBlockTexts(specs, []string{"same"}); len(missing) != 1 {
		t.Fatalf("one recovered block must not satisfy two expectations, got %v", missing)
	}
	if missing := missingCodeBlockTexts(specs, []string{"same", "same"}); len(missing) != 0 {
		t.Fatalf("both occurrences present should verify, got %v", missing)
	}
}

func TestMissingCodeBlockTextsIgnoresLineEndingAndTrailingSpaceDifferences(t *testing.T) {
	specs := []Spec{{Kind: "code", Text: "line one\nline two"}}
	if missing := missingCodeBlockTexts(specs, []string{"line one   \r\nline two\n"}); len(missing) != 0 {
		t.Fatalf("normalized differences should verify, got %v", missing)
	}
}

func TestMissingCodeBlockTextsIsEmptyWithoutCodeSpecs(t *testing.T) {
	specs := []Spec{{Kind: "text", Text: "no code here"}}
	if missing := missingCodeBlockTexts(specs, []string{"stray"}); missing != nil {
		t.Fatalf("no code specs means nothing to verify, got %v", missing)
	}
}

// An empty fenced block carries no text to compare, so it must not be reported as
// missing when the document legitimately has nothing to recover.
func TestMissingCodeBlockTextsSkipsEmptyCodeSpecs(t *testing.T) {
	specs := []Spec{{Kind: "code", Text: "   \n  "}}
	if missing := missingCodeBlockTexts(specs, nil); len(missing) != 0 {
		t.Fatalf("an empty code block should not be reported missing, got %v", missing)
	}
}
