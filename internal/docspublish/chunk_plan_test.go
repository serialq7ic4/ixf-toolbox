package docspublish

import (
	"strings"
	"testing"
)

func TestPartitionWriteGroupsKeepsSmallDocumentInOneWrite(t *testing.T) {
	specs := []Spec{
		{Kind: "heading2", Text: "Section"},
		{Kind: "text", Text: "Paragraph"},
	}
	groups, err := partitionWriteGroups(specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected a single write, got %d", len(groups))
	}
	if groups[0].StartSpec != 0 || groups[0].EndSpec != len(specs) {
		t.Fatalf("expected the group to span all specs, got %+v", groups[0])
	}
	if groups[0].StartEntry != 0 || groups[0].EndEntry != 2 {
		t.Fatalf("expected entry range [0,2), got [%d,%d)", groups[0].StartEntry, groups[0].EndEntry)
	}
}

func TestPartitionWriteGroupsSplitsOverBudgetDocument(t *testing.T) {
	specs := make([]Spec, blockEntryWriteBudget+10)
	for index := range specs {
		specs[index] = Spec{Kind: "text", Text: "p"}
	}
	groups, err := partitionWriteGroups(specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(groups))
	}
	for _, group := range groups {
		if group.BlockEntries > blockEntryWriteBudget {
			t.Fatalf("group carries %d entries, over the %d budget", group.BlockEntries, blockEntryWriteBudget)
		}
		if changeEntryCount(group.BlockEntries) > maxChangeEntriesPerWrite {
			t.Fatalf("group would send %d change entries, over the %d limit",
				changeEntryCount(group.BlockEntries), maxChangeEntriesPerWrite)
		}
	}
}

// Groups must tile the spec and entry ranges exactly: no gaps (dropped content)
// and no overlaps (duplicated content).
func TestPartitionWriteGroupsTilesRangesContiguously(t *testing.T) {
	specs := make([]Spec, blockEntryWriteBudget*2+5)
	for index := range specs {
		specs[index] = Spec{Kind: "text", Text: "p"}
	}
	groups, err := partitionWriteGroups(specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) < 3 {
		t.Fatalf("expected at least 3 writes, got %d", len(groups))
	}
	expectedSpec, expectedEntry := 0, 0
	for index, group := range groups {
		if group.StartSpec != expectedSpec {
			t.Fatalf("group %d starts at spec %d, expected %d", index, group.StartSpec, expectedSpec)
		}
		if group.StartEntry != expectedEntry {
			t.Fatalf("group %d starts at entry %d, expected %d", index, group.StartEntry, expectedEntry)
		}
		expectedSpec, expectedEntry = group.EndSpec, group.EndEntry
	}
	if expectedSpec != len(specs) {
		t.Fatalf("groups cover %d specs, want %d", expectedSpec, len(specs))
	}
	totalEntries, _ := measureSpecs(specs)
	if expectedEntry != totalEntries {
		t.Fatalf("groups cover %d entries, want %d", expectedEntry, totalEntries)
	}
}

// A single top-level spec bigger than one write cannot be split by any grouping,
// so it must be refused up front rather than sent and rejected.
func TestPartitionWriteGroupsRejectsIndivisibleSpec(t *testing.T) {
	rows := make([][]string, blockEntryWriteBudget)
	for index := range rows {
		rows[index] = []string{"a", "b", "c"}
	}
	_, err := partitionWriteGroups([]Spec{{Kind: "table", Rows: rows}})
	if err == nil {
		t.Fatal("expected an error for a table that cannot fit in one write")
	}
	if !strings.Contains(err.Error(), "table") {
		t.Fatalf("error should name the offending block kind, got %v", err)
	}
	if !strings.Contains(err.Error(), "no split can make it fit") {
		t.Fatalf("error should explain that splitting cannot help, got %v", err)
	}
}

func TestPartitionWriteGroupsHandlesEmptySpecs(t *testing.T) {
	groups, err := partitionWriteGroups(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected no groups for no specs, got %d", len(groups))
	}
}

// The highest-ranked correctness risk: a boundary must never separate an ordered
// item from the code block it owns as a child. Splitting on top-level specs makes
// that structurally impossible, and this pins the behavior.
func TestPartitionWriteGroupsNeverSeparatesParentFromChildren(t *testing.T) {
	filler := make([]Spec, blockEntryWriteBudget-1)
	for index := range filler {
		filler[index] = Spec{Kind: "text", Text: "p"}
	}
	ordered := Spec{
		Kind:     "ordered",
		Text:     "step one",
		Children: []Spec{{Kind: "code", Text: "echo hi"}},
	}
	specs := append(append([]Spec{}, filler...), ordered, Spec{Kind: "text", Text: "tail"})

	groups, err := partitionWriteGroups(specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) < 2 {
		t.Fatalf("expected the document to split, got %d writes", len(groups))
	}

	orderedIndex := len(filler)
	_, entries := buildBlocks(specs, "page", newBlockFactory("author"))
	for _, group := range groups {
		if orderedIndex < group.StartSpec || orderedIndex >= group.EndSpec {
			continue
		}
		// The ordered item landed in this group; its child entry must be here too.
		slice := entries[group.StartEntry:group.EndEntry]
		ids := map[string]bool{}
		for _, entry := range slice {
			ids[entry.ID] = true
		}
		found := false
		for _, entry := range slice {
			parent := asString(asMap(entry.Data)["parent_id"])
			if parent != "" && ids[parent] && parent != "page" {
				found = true
			}
		}
		if !found {
			t.Fatal("the ordered item's child block was not written in the same group as its parent")
		}
		return
	}
	t.Fatal("did not find the group containing the ordered item")
}
