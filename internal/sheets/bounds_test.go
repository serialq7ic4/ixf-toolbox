package sheets

import (
	"strings"
	"testing"
)

// The read window is wider than the data, so trailing empty rows must not count as
// populated. This is the misreading that made the issue author think a write had
// grown the sheet.
func TestLastPopulatedRowIgnoresTrailingEmptyRows(t *testing.T) {
	values := [][]string{
		{"a", "b"},
		{"c", ""},
		{"", ""},
		{"", ""},
	}
	if got := lastPopulatedRow(values); got != 1 {
		t.Fatalf("last populated row = %d, want 1", got)
	}
}

func TestLastPopulatedRowReportsEmptySheet(t *testing.T) {
	if got := lastPopulatedRow([][]string{{"", ""}, {""}}); got != -1 {
		t.Fatalf("an empty sheet should report -1, got %d", got)
	}
	if got := lastPopulatedRow(nil); got != -1 {
		t.Fatalf("no rows should report -1, got %d", got)
	}
}

// The reported case: 12 populated rows, write starting at row 30.
func TestRangeGapWarningFlagsWriteFarBelowData(t *testing.T) {
	values := make([][]string, 40)
	for index := range values {
		if index < 12 {
			values[index] = []string{"data"}
			continue
		}
		values[index] = []string{""}
	}
	warning := rangeGapWarning(values, 29, 1)
	if warning == "" {
		t.Fatal("expected a warning for a start row far below the data")
	}
	if !strings.Contains(warning, "row 30") {
		t.Fatalf("warning should name the 1-based start row, got %q", warning)
	}
	if !strings.Contains(warning, "12") {
		t.Fatalf("warning should name the last populated row, got %q", warning)
	}
	if !strings.Contains(warning, "17") {
		t.Fatalf("warning should name the 17-row gap, got %q", warning)
	}
}

// Writing inside the data is the ordinary case and must stay silent.
func TestRangeGapWarningSilentWhenWritingInsideData(t *testing.T) {
	values := [][]string{{"a"}, {"b"}, {"c"}, {"d"}}
	if warning := rangeGapWarning(values, 1, 2); warning != "" {
		t.Fatalf("writing inside the data should not warn, got %q", warning)
	}
}

// Appending on the first row after the data is deliberate extension, not a mistake.
func TestRangeGapWarningSilentWhenAppendingDirectlyAfterData(t *testing.T) {
	values := [][]string{{"a"}, {"b"}, {"", ""}, {"", ""}}
	if warning := rangeGapWarning(values, 2, 1); warning != "" {
		t.Fatalf("appending immediately after the data should not warn, got %q", warning)
	}
}

func TestRangeGapWarningSilentOnEmptySheet(t *testing.T) {
	if warning := rangeGapWarning([][]string{{""}, {""}}, 5, 1); warning != "" {
		t.Fatalf("an empty sheet has no reference row, so no warning: got %q", warning)
	}
}

// One skipped row is the smallest real off-by-one and must still be reported.
func TestRangeGapWarningFlagsSingleRowGap(t *testing.T) {
	values := [][]string{{"a"}, {"b"}, {""}, {""}}
	warning := rangeGapWarning(values, 3, 1)
	if warning == "" {
		t.Fatal("expected a warning when exactly one row is skipped")
	}
	if !strings.Contains(warning, "1 row(s)") {
		t.Fatalf("warning should name a one-row gap, got %q", warning)
	}
}
