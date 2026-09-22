package sheets

import (
	"fmt"
	"strings"
)

// validateTSVInput rejects an --input file that the TSV reader cannot map onto a
// multi-column target range.
//
// readTSV splits each line on tabs only, so a file in any other format parses
// without error into single-column rows. A JSON array of pairs, for example,
// becomes one cell holding the literal text `[["a","b"]]`, which then writes into
// the first target column and leaves the neighbouring column untouched. Nothing
// downstream catches it: the dry run reports the resulting cols:1 without comment
// and the post-write check compares stored values against what was sent, so it
// passes. Failing here is the only place the mistake is still cheap.
func validateTSVInput(path string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	if kind := structuredFormatName(rows); kind != "" {
		return fmt.Errorf(
			"--input %s looks like %s, but this command reads tab-separated values:"+
				" one line per row, columns separated by a literal tab."+
				" Convert the data, for example `printf 'colA\\tcolB\\n' > cells.tsv`",
			path, kind)
	}
	return nil
}

// structuredFormatName names the format an input file appears to be in when it is
// clearly not TSV, or "" when the content is plausibly tab-separated.
//
// A single-column file is legitimate when the target really is one column, so the
// check does not reject on column count alone. It reports only content whose shape
// contradicts TSV outright.
func structuredFormatName(rows [][]string) string {
	if maxColumns(rows) > 1 {
		return ""
	}
	first := ""
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if candidate := strings.TrimSpace(row[0]); candidate != "" {
			first = candidate
			break
		}
	}
	if first == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(first, "[") || strings.HasPrefix(first, "{"):
		return "JSON"
	case strings.HasPrefix(first, "---") && len(rows) > 1:
		return "YAML"
	case strings.Count(first, ",") > 0 && looksDelimited(rows, ","):
		return "comma-separated values"
	case strings.Count(first, ";") > 0 && looksDelimited(rows, ";"):
		return "semicolon-separated values"
	}
	return ""
}

// looksDelimited reports whether every non-empty row carries the same number of
// the given separator, which is what a delimited file looks like and what an
// ordinary sentence containing one comma does not.
func looksDelimited(rows [][]string, separator string) bool {
	count := -1
	for _, row := range rows {
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		found := strings.Count(row[0], separator)
		if found == 0 {
			return false
		}
		if count == -1 {
			count = found
			continue
		}
		if found != count {
			return false
		}
	}
	return count > 0
}

func maxColumns(rows [][]string) int {
	widest := 0
	for _, row := range rows {
		if len(row) > widest {
			widest = len(row)
		}
	}
	return widest
}
