package docxgraph

import "testing"

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
