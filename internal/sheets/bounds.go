package sheets

import "fmt"

// lastPopulatedRow reports the 0-based index of the last row holding any non-empty
// cell, or -1 when the sheet has no content.
//
// The read window is wider than the data: trailing rows come back as empty strings,
// so the length of the values slice overstates how far the content reaches. Only a
// scan tells the two apart.
func lastPopulatedRow(values [][]string) int {
	last := -1
	for rowIndex, row := range values {
		for _, cell := range row {
			if cell != "" {
				last = rowIndex
				break
			}
		}
	}
	return last
}

// rangeGapWarning reports a start row that sits past the end of the populated data,
// leaving a gap of untouched rows between the content and the write.
//
// This is a warning rather than an error: writing below existing content is a
// legitimate way to extend a sheet. But an off-by-N start cell looks identical to
// that intent at the API level, and nothing else in the command distinguishes them,
// so the gap is worth naming. Writing into or adjacent to the data produces no
// warning.
func rangeGapWarning(values [][]string, startRow int, rowCount int) string {
	last := lastPopulatedRow(values)
	if last < 0 || startRow <= last+1 {
		return ""
	}
	gap := startRow - last - 1
	return fmt.Sprintf(
		"start row %d begins %d row(s) below the last populated row %d, so %d row(s) between them are left untouched;"+
			" confirm the start cell if this was meant to land inside the existing data",
		startRow+1, gap, last+1, gap)
}
