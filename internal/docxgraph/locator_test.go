package docxgraph

import "testing"

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

func TestNormalizeHeadingCollapsesWhitespaceAndFullWidthSpaces(t *testing.T) {
	got := NormalizeHeading("  1.1　账号   全集群\t初始化  ")
	want := "1.1 账号 全集群 初始化"
	if got != want {
		t.Fatalf("normalized heading = %q, want %q", got, want)
	}
}

func TestFindHeadingByTextRejectsDuplicateMatches(t *testing.T) {
	graph := graphFixtureWithRootChildren(
		blockFixture("h1", "heading2", "Duplicate"),
		blockFixture("h2", "heading3", " Duplicate "),
	)
	_, err := graph.FindHeadingByText("Duplicate")
	if err == nil {
		t.Fatal("expected duplicate heading error")
	}
}
