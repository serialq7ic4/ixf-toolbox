package docspublish

import (
	"testing"

	"github.com/serialq7ic4/ixf-toolbox/internal/docxgraph"
)

// appendVerifyGraph builds a graph holding one table whose rows and cell_set are
// exactly as given, so each verification failure can be constructed directly.
func appendVerifyGraph(rowIDs []any, cellSet map[string]any, cells map[string]string) docxgraph.Graph {
	blocks := map[string]docxgraph.Block{
		"table_1": {
			ID:   "table_1",
			Kind: "table",
			Raw: map[string]any{
				"rows_id":    rowIDs,
				"columns_id": []any{"col_a", "col_b"},
				"cell_set":   cellSet,
			},
		},
	}
	for cellID, text := range cells {
		textID := cellID + "_text"
		blocks[cellID] = docxgraph.Block{
			ID: cellID, Kind: "table_cell", ParentID: "table_1", Children: []string{textID},
		}
		blocks[textID] = docxgraph.Block{ID: textID, Kind: "text", ParentID: cellID, Text: text}
	}
	return docxgraph.Graph{RootID: "page_1", Blocks: blocks}
}

func cellSetFor(rowID string, cellA string, cellB string) map[string]any {
	return map[string]any{
		rowID + "col_a": map[string]any{"block_id": cellA},
		rowID + "col_b": map[string]any{"block_id": cellB},
	}
}

func TestVerifyAppendedRowAcceptsACorrectAppend(t *testing.T) {
	graph := appendVerifyGraph(
		[]any{"row_header", "row_new"},
		cellSetFor("row_new", "cell_a", "cell_b"),
		map[string]string{"cell_a": "alpha", "cell_b": "beta"},
	)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != true {
		t.Fatalf("a correct append should verify, got %+v", result)
	}
	if result["rowsAfter"] != 2 {
		t.Fatalf("rowsAfter = %v, want 2", result["rowsAfter"])
	}
}

// The issue #18 case: the write did not land. The old check passed because the sent
// text existed elsewhere in the document; this one reports the row count.
func TestVerifyAppendedRowRejectsWhenNoRowWasAdded(t *testing.T) {
	graph := appendVerifyGraph([]any{"row_header"}, cellSetFor("row_header", "cell_h_a", "cell_h_b"), nil)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != false {
		t.Fatal("expected verification to fail when the row count did not grow")
	}
	if result["rowsAfter"] != 1 {
		t.Fatalf("rowsAfter = %v, want 1", result["rowsAfter"])
	}
}

// A row that exists but is not last means ordering was not preserved.
func TestVerifyAppendedRowRejectsRowThatIsNotLast(t *testing.T) {
	graph := appendVerifyGraph(
		[]any{"row_new", "row_header"},
		cellSetFor("row_new", "cell_a", "cell_b"),
		map[string]string{"cell_a": "alpha", "cell_b": "beta"},
	)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != false {
		t.Fatal("expected verification to fail when the new row is not last")
	}
}

// Two rows appearing where one was appended means something else also wrote.
func TestVerifyAppendedRowRejectsMoreThanOneNewRow(t *testing.T) {
	graph := appendVerifyGraph(
		[]any{"row_header", "row_other", "row_new"},
		cellSetFor("row_new", "cell_a", "cell_b"),
		map[string]string{"cell_a": "alpha", "cell_b": "beta"},
	)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != false {
		t.Fatal("expected verification to fail when the row count grew by more than one")
	}
}

// Cells collapsing into one column is the exact shape issue #8 produced in sheets
// and the substring check could not see here.
func TestVerifyAppendedRowRejectsMissingCellRegistration(t *testing.T) {
	cellSet := map[string]any{"row_newcol_a": map[string]any{"block_id": "cell_a"}}
	graph := appendVerifyGraph([]any{"row_header", "row_new"}, cellSet, map[string]string{"cell_a": "alpha"})
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != false {
		t.Fatal("expected verification to fail when a column has no registered cell")
	}
	missing, _ := result["missingCells"].([]map[string]any)
	if len(missing) != 1 || missing[0]["column"] != 2 {
		t.Fatalf("expected column 2 to be reported missing, got %+v", result["missingCells"])
	}
}

func TestVerifyAppendedRowRejectsWrongCellText(t *testing.T) {
	graph := appendVerifyGraph(
		[]any{"row_header", "row_new"},
		cellSetFor("row_new", "cell_a", "cell_b"),
		map[string]string{"cell_a": "alpha", "cell_b": "something else"},
	)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != false {
		t.Fatal("expected verification to fail when cell text does not match")
	}
}

// A different cell block registered for the column means the write was reshaped.
func TestVerifyAppendedRowRejectsUnexpectedCellBlock(t *testing.T) {
	graph := appendVerifyGraph(
		[]any{"row_header", "row_new"},
		cellSetFor("row_new", "cell_a", "someone_elses_cell"),
		map[string]string{"cell_a": "alpha", "someone_elses_cell": "beta"},
	)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_b"}, []string{"alpha", "beta"})
	if result["ok"] != false {
		t.Fatal("expected verification to fail when another cell block holds the column")
	}
}

// An image cell carries no text, so it must not be required to match one.
func TestVerifyAppendedRowSkipsTextCheckForImageCells(t *testing.T) {
	graph := appendVerifyGraph(
		[]any{"row_header", "row_new"},
		cellSetFor("row_new", "cell_a", "cell_img"),
		map[string]string{"cell_a": "alpha", "cell_img": ""},
	)
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a", "cell_img"}, []string{"alpha", ""})
	if result["ok"] != true {
		t.Fatalf("an image cell should not require matching text, got %+v", result)
	}
}

func TestVerifyAppendedRowReportsMissingTable(t *testing.T) {
	graph := docxgraph.Graph{RootID: "page_1", Blocks: map[string]docxgraph.Block{}}
	result := verifyAppendedRow(graph, "table_1", 1, "row_new", []string{"cell_a"}, []string{"alpha"})
	if result["ok"] != false || result["error"] == nil {
		t.Fatalf("expected a missing table to be reported, got %+v", result)
	}
}
