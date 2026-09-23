package okr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSpecFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "okr.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The issue #17 regression. A misspelled key decoded cleanly into zero KRs, which
// then deleted every existing KR and created nothing.
func TestParseSpecsRejectsMisspelledKRKey(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1","key_results":["a","b"]}]}`)
	_, err := ParseSpecs(path)
	if err == nil {
		t.Fatal("expected a misspelled krs key to be rejected")
	}
	if !strings.Contains(err.Error(), "key_results") {
		t.Fatalf("error should name the unknown field, got %v", err)
	}
}

func TestParseSpecsRejectsCamelCaseKRKey(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1","keyResults":["a"]}]}`)
	if _, err := ParseSpecs(path); err == nil {
		t.Fatal("expected keyResults to be rejected")
	}
}

func TestParseSpecsRejectsEmptyKRList(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1","krs":[]}]}`)
	_, err := ParseSpecs(path)
	if err == nil {
		t.Fatal("expected an empty krs list to be rejected")
	}
	if !strings.Contains(err.Error(), "delete") {
		t.Fatalf("error should explain the deletion consequence, got %v", err)
	}
}

func TestParseSpecsRejectsAbsentKRKey(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1"}]}`)
	if _, err := ParseSpecs(path); err == nil {
		t.Fatal("expected an absent krs key to be rejected")
	}
}

// Trimming can reduce a non-empty list to nothing, which would reach the write path
// as an empty replacement just like an absent key.
func TestParseSpecsRejectsAllBlankKRs(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1","krs":["  ","\t"]}]}`)
	_, err := ParseSpecs(path)
	if err == nil {
		t.Fatal("expected all-blank KRs to be rejected")
	}
	if !strings.Contains(err.Error(), "blank") {
		t.Fatalf("error should say the KRs were blank rather than absent, got %v", err)
	}
}

// The README documented a top-level array, which never parsed. The error should
// point at the shape that does.
func TestParseSpecsRejectsTopLevelArrayWithShapeHint(t *testing.T) {
	path := writeSpecFile(t, `[{"objective":"O1","krs":["a"]}]`)
	_, err := ParseSpecs(path)
	if err == nil {
		t.Fatal("expected a top-level array to be rejected")
	}
	if !strings.Contains(err.Error(), `{"objectives"`) {
		t.Fatalf("error should show the accepted shape, got %v", err)
	}
}

func TestParseSpecsAcceptsValidInput(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1","krs":["a","b"]}]}`)
	specs, err := ParseSpecs(path)
	if err != nil {
		t.Fatalf("valid input should parse, got %v", err)
	}
	if len(specs) != 1 || len(specs[0].KRs) != 2 {
		t.Fatalf("unexpected parse result: %+v", specs)
	}
}

func TestParseSpecsKeepsExistingCeiling(t *testing.T) {
	path := writeSpecFile(t, `{"objectives":[{"objective":"O1","krs":["a","b","c","d","e"]}]}`)
	if _, err := ParseSpecs(path); err == nil {
		t.Fatal("expected the 4-KR ceiling to still apply")
	}
}
