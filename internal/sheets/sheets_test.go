package sheets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTargetRequiresDirectSheetURL(t *testing.T) {
	target, err := ParseTarget("https://tenant.example.test/sheets/shtr_fixture?sheet=sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if target.BaseURL != "https://tenant.example.test" || target.WorkbookToken != "shtr_fixture" || target.SheetID != "sheet1" {
		t.Fatalf("target = %+v", target)
	}

	if _, err := ParseTarget("https://tenant.example.test/docx/dox_fixture"); err == nil {
		t.Fatal("ParseTarget accepted non-sheets URL")
	}
	if _, err := ParseTarget("https://tenant.example.test/sheets/shtr_fixture"); err == nil {
		t.Fatal("ParseTarget accepted missing sheet query")
	}
}

func TestPlanUpdateDryRunReportsTSVShape(t *testing.T) {
	tmpDir := t.TempDir()
	input := filepath.Join(tmpDir, "cells.tsv")
	if err := os.WriteFile(input, []byte("A\tB\n1\t2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := PlanUpdate(UpdateConfig{
		URL:       "https://tenant.example.test/sheets/shtr_fixture?sheet=sheet1",
		Range:     "b2",
		InputPath: input,
		DryRun:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if payload["range"] != "B2" || payload["rows"] != 2 || payload["cols"] != 2 || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPlanUpdateApplyIsExplicitlyUnavailable(t *testing.T) {
	_, err := PlanUpdate(UpdateConfig{
		URL:       "https://tenant.example.test/sheets/shtr_fixture?sheet=sheet1",
		Range:     "A1",
		InputPath: "cells.tsv",
		Apply:     true,
	})
	if err == nil {
		t.Fatal("PlanUpdate accepted --apply without a write API contract")
	}
}
