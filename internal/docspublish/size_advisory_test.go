package docspublish

import (
	"strings"
	"testing"
)

// The server counts the page root entry, so the reported change entry count must
// be one above the block entry count. Getting this wrong understates the write by
// exactly one at the boundary that was measured.
func TestChangeEntryCountIncludesPageRoot(t *testing.T) {
	if got := changeEntryCount(3499); got != maxChangeEntriesPerWrite {
		t.Fatalf("3499 block entries should be %d change entries, got %d", maxChangeEntriesPerWrite, got)
	}
	if maxBlockEntriesPerWrite != maxChangeEntriesPerWrite-1 {
		t.Fatalf("block ceiling %d should be one below the change ceiling %d",
			maxBlockEntriesPerWrite, maxChangeEntriesPerWrite)
	}
}

func TestWriteBudgetStaysUnderMeasuredCeiling(t *testing.T) {
	if blockEntryWriteBudget > maxBlockEntriesPerWrite {
		t.Fatalf("write budget %d must not exceed the measured block ceiling %d",
			blockEntryWriteBudget, maxBlockEntriesPerWrite)
	}
	if changeEntryCount(blockEntryWriteBudget) > maxChangeEntriesPerWrite {
		t.Fatalf("a full budget write is %d change entries, above the %d limit",
			changeEntryCount(blockEntryWriteBudget), maxChangeEntriesPerWrite)
	}
}

func TestSizeAdvisoryReportsMeasurementsAndLimit(t *testing.T) {
	payload := sizeAdvisory{BlockEntries: 12, PayloadBytes: 3456, WriteCount: 1}.applyTo(map[string]any{})
	if payload["plannedBlockEntries"] != 12 {
		t.Fatalf("plannedBlockEntries = %v, want 12", payload["plannedBlockEntries"])
	}
	if payload["plannedChangeEntries"] != 13 {
		t.Fatalf("plannedChangeEntries = %v, want 13", payload["plannedChangeEntries"])
	}
	if payload["plannedPayloadBytes"] != 3456 {
		t.Fatalf("plannedPayloadBytes = %v, want 3456", payload["plannedPayloadBytes"])
	}
	if payload["maxChangeEntriesPerWrite"] != maxChangeEntriesPerWrite {
		t.Fatalf("expected the measured limit to be reported")
	}
	if _, ok := payload["willSplitWrites"]; ok {
		t.Fatal("a single-write document must not be marked as split")
	}
}

// Payload bytes are reported but never thresholded: 3.0MB in 200 blocks published
// fine while 1.4MB in 3860 blocks failed.
func TestSizeAdvisoryDoesNotThresholdPayloadBytes(t *testing.T) {
	payload := sizeAdvisory{BlockEntries: 200, PayloadBytes: 3 * 1024 * 1024, WriteCount: 1}.applyTo(map[string]any{})
	if _, ok := payload["willSplitWrites"]; ok {
		t.Fatal("a 3MB write in 200 blocks must not be flagged; the limit is on entry count")
	}
	if payload["plannedPayloadBytes"] != 3*1024*1024 {
		t.Fatal("payload bytes should still be reported as diagnostics")
	}
}

func TestSizeAdvisoryMarksSplitWrites(t *testing.T) {
	payload := sizeAdvisory{BlockEntries: 8000, PayloadBytes: 1024, WriteCount: 3}.applyTo(map[string]any{})
	if payload["willSplitWrites"] != true {
		t.Fatal("expected willSplitWrites for a multi-write document")
	}
	if payload["plannedWriteCount"] != 3 {
		t.Fatalf("plannedWriteCount = %v, want 3", payload["plannedWriteCount"])
	}
	reason, _ := payload["splitReason"].(string)
	if !strings.Contains(reason, "3 sequential writes") {
		t.Fatalf("splitReason should state the write count, got %q", reason)
	}
}

func TestWriteSizeHintStatesTheCauseAboveTheLimit(t *testing.T) {
	hint := writeSizeHint(maxChangeEntriesPerWrite + 1)
	if !strings.Contains(hint, "count limit, not a byte limit") {
		t.Fatalf("expected a definite count-limit diagnosis, got %q", hint)
	}
	if !strings.Contains(hint, "3500") {
		t.Fatalf("expected the hint to name the limit, got %q", hint)
	}
}

// At or below the limit the cause is something else, so the hint must not blame
// size and misdirect the reader.
func TestWriteSizeHintDeclinesToBlameSizeWithinTheLimit(t *testing.T) {
	hint := writeSizeHint(120)
	if !strings.Contains(hint, "probably not the cause") {
		t.Fatalf("expected the hint to decline blaming size, got %q", hint)
	}
}

func TestMeasureSpecsCountsExpandedBlockEntries(t *testing.T) {
	specs := []Spec{
		{Kind: "heading2", Text: "Section"},
		{Kind: "text", Text: "Paragraph"},
	}
	entries, payloadBytes := measureSpecs(specs)
	if entries != len(specs) {
		t.Fatalf("expected %d entries for flat specs, got %d", len(specs), entries)
	}
	if payloadBytes <= 0 {
		t.Fatalf("expected positive payload bytes, got %d", payloadBytes)
	}

	table := []Spec{{Kind: "table", Rows: [][]string{{"a", "b"}, {"c", "d"}}}}
	tableEntries, _ := measureSpecs(table)
	if tableEntries <= len(table) {
		t.Fatalf("expected a table to expand beyond %d entries, got %d", len(table), tableEntries)
	}
}

func TestMeasureSpecsIsDeterministic(t *testing.T) {
	specs := []Spec{{Kind: "text", Text: "Paragraph"}}
	firstEntries, firstBytes := measureSpecs(specs)
	secondEntries, secondBytes := measureSpecs(specs)
	if firstEntries != secondEntries || firstBytes != secondBytes {
		t.Fatalf("expected deterministic measurement, got (%d,%d) then (%d,%d)",
			firstEntries, firstBytes, secondEntries, secondBytes)
	}
}
