package sheets

import (
	"strings"
	"testing"
)

// The issue #8 regression: a JSON array of pairs parses as one single-column row
// and previously wrote its literal text into the first target column.
func TestValidateTSVInputRejectsJSON(t *testing.T) {
	rows := [][]string{{`[["evidence text","assessment text"]]`}}
	err := validateTSVInput("cells.json", rows)
	if err == nil {
		t.Fatal("expected JSON input to be rejected")
	}
	if !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("error should name the detected format, got %v", err)
	}
	if !strings.Contains(err.Error(), "tab-separated") {
		t.Fatalf("error should state the expected format, got %v", err)
	}
	if !strings.Contains(err.Error(), "cells.json") {
		t.Fatalf("error should name the offending file, got %v", err)
	}
}

func TestValidateTSVInputRejectsJSONObject(t *testing.T) {
	rows := [][]string{{`{"a": 1}`}}
	if err := validateTSVInput("cells.json", rows); err == nil {
		t.Fatal("expected a JSON object to be rejected")
	}
}

func TestValidateTSVInputAcceptsTabSeparatedRows(t *testing.T) {
	rows := [][]string{{"evidence text", "assessment text"}, {"a", "b"}}
	if err := validateTSVInput("cells.tsv", rows); err != nil {
		t.Fatalf("valid TSV should be accepted, got %v", err)
	}
}

// A single column is a legitimate target, so column count alone must not reject.
func TestValidateTSVInputAcceptsLegitimateSingleColumn(t *testing.T) {
	rows := [][]string{{"first note"}, {"second note"}, {"third note"}}
	if err := validateTSVInput("notes.tsv", rows); err != nil {
		t.Fatalf("a single-column file should be accepted, got %v", err)
	}
}

func TestValidateTSVInputRejectsCSV(t *testing.T) {
	rows := [][]string{{"a,b"}, {"c,d"}}
	err := validateTSVInput("cells.csv", rows)
	if err == nil {
		t.Fatal("expected consistently comma-delimited input to be rejected")
	}
	if !strings.Contains(err.Error(), "comma-separated") {
		t.Fatalf("error should name CSV, got %v", err)
	}
}

// Prose legitimately contains commas. Only consistent delimiting looks like CSV,
// so a sentence with one comma must still be accepted as a single cell.
func TestValidateTSVInputAcceptsProseContainingCommas(t *testing.T) {
	rows := [][]string{{"first, with a comma"}, {"second line with no delimiter at all"}}
	if err := validateTSVInput("notes.tsv", rows); err != nil {
		t.Fatalf("prose containing a comma should be accepted, got %v", err)
	}
}

func TestValidateTSVInputAcceptsUnevenCommaCounts(t *testing.T) {
	rows := [][]string{{"a,b,c"}, {"d,e"}}
	if err := validateTSVInput("notes.tsv", rows); err != nil {
		t.Fatalf("inconsistent comma counts are not CSV, got %v", err)
	}
}

func TestValidateTSVInputAcceptsEmptyRows(t *testing.T) {
	if err := validateTSVInput("empty.tsv", nil); err != nil {
		t.Fatalf("no rows should not error here, got %v", err)
	}
}

// A value that merely starts with a bracket inside a proper two-column row is real
// data, not a format mistake.
func TestValidateTSVInputAcceptsBracketedTextInMultiColumnRow(t *testing.T) {
	rows := [][]string{{"[draft] heading", "assessment"}}
	if err := validateTSVInput("cells.tsv", rows); err != nil {
		t.Fatalf("bracketed text in a real TSV row should be accepted, got %v", err)
	}
}
