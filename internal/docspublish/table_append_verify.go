package docspublish

import (
	"fmt"

	"github.com/serialq7ic4/ixf-toolbox/internal/docxgraph"
)

// verifyAppendedRow confirms the new row actually exists in the target table, with
// its cells in the expected columns holding the expected text.
//
// The previous check asked whether each cell's text appeared anywhere in the
// document, using the values that had just been sent as the expectation. That
// passed whenever the values already occurred elsewhere, which is the normal case
// for low-cardinality columns such as a status or an owner, so a write that was
// rejected, landed in a different table, or collapsed its cells into one verified
// exactly as faithfully as a correct one. Nothing checked that the table had grown.
func verifyAppendedRow(
	graph docxgraph.Graph,
	tableID string,
	rowsBefore int,
	rowID string,
	cellIDs []string,
	expectedTexts []string,
) map[string]any {
	result := map[string]any{
		"ok":            false,
		"rowsBefore":    rowsBefore,
		"appendedRowId": rowID,
		"scope":         "structural: the new row exists in the target table, is its last child, and its cells hold the expected text in the expected columns",
	}

	table, ok := graph.Blocks[tableID]
	if !ok {
		result["error"] = fmt.Sprintf("table %s was not found after writing", tableID)
		return result
	}
	// A table's rows live in its raw `rows_id`, not in Children, which is how the
	// rest of this package reads them.
	rowIDs := stringSlice(asSlice(table.Raw["rows_id"]))
	rowsAfter := len(rowIDs)
	result["rowsAfter"] = rowsAfter

	if rowsAfter != rowsBefore+1 {
		result["error"] = fmt.Sprintf("table row count went from %d to %d; expected exactly one new row",
			rowsBefore, rowsAfter)
		return result
	}
	if rowsAfter == 0 || rowIDs[rowsAfter-1] != rowID {
		result["error"] = fmt.Sprintf("row %s is not the last row of table %s after writing", rowID, tableID)
		return result
	}

	columnIDs := stringSlice(asSlice(table.Raw["columns_id"]))
	cellSet := asMap(table.Raw["cell_set"])
	missing := verifyAppendedCells(graph, cellSet, rowID, columnIDs, cellIDs, expectedTexts)
	result["cellsChecked"] = len(cellIDs)
	if len(missing) > 0 {
		result["missingCells"] = missing
		result["error"] = fmt.Sprintf("%d of %d cells did not hold the expected text", len(missing), len(cellIDs))
		return result
	}

	result["ok"] = true
	return result
}

// verifyAppendedCells reports the columns whose cell is missing, misparented, or
// holding text other than what was written. Column indexes are 1-based to match
// how the headers are presented to the caller.
// verifyAppendedCells reports the columns whose cell is not registered against the
// new row, is missing as a block, or holds text other than what was written.
//
// A cell is reached through the table's cell_set, keyed by row id concatenated with
// column id, rather than through the block tree: a cell block's parent is the table
// itself, so parentage alone cannot tell which row a cell belongs to. Checking the
// cell_set entry is what confirms the cell landed in the intended column of the
// intended row. Column numbers are 1-based to match how headers are reported.
func verifyAppendedCells(
	graph docxgraph.Graph,
	cellSet map[string]any,
	rowID string,
	columnIDs []string,
	cellIDs []string,
	expectedTexts []string,
) []map[string]any {
	missing := []map[string]any{}
	for index, cellID := range cellIDs {
		column := index + 1
		if index >= len(columnIDs) {
			missing = append(missing, map[string]any{"column": column, "reason": "table has fewer columns than the row was built for"})
			continue
		}
		registered := asString(asMap(cellSet[rowID+columnIDs[index]])["block_id"])
		if registered == "" {
			missing = append(missing, map[string]any{"column": column, "reason": "no cell is registered for this column of the appended row"})
			continue
		}
		if registered != cellID {
			missing = append(missing, map[string]any{"column": column, "reason": "a different cell block is registered for this column"})
			continue
		}
		if _, ok := graph.Blocks[cellID]; !ok {
			missing = append(missing, map[string]any{"column": column, "reason": "cell block not found"})
			continue
		}
		if index >= len(expectedTexts) || expectedTexts[index] == "" {
			continue
		}
		if cellSubtreeText(graph, cellID) != expectedTexts[index] {
			missing = append(missing, map[string]any{"column": column, "reason": "cell text does not match what was written"})
		}
	}
	return missing
}

// cellSubtreeText collects the text of a cell's descendants. A cell holds its
// content in child blocks rather than directly, so the cell's own Text is empty.
func cellSubtreeText(graph docxgraph.Graph, cellID string) string {
	text := ""
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if depth > 8 {
			return
		}
		block, ok := graph.Blocks[id]
		if !ok {
			return
		}
		text += block.Text
		for _, child := range block.Children {
			walk(child, depth+1)
		}
	}
	for _, child := range graph.Blocks[cellID].Children {
		walk(child, 0)
	}
	return text
}
