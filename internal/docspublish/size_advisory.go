package docspublish

import (
	"encoding/json"
	"fmt"
)

// Write-size limits for a single docx block write.
//
// Measured against the live editor endpoint on 2026-09-21 by bisection: a write
// whose change_map holds 3500 entries succeeds, and 3501 is rejected with
// `code=4000002: invalid param`. The change_map always carries one entry for the
// page root plus one entry per block, so 3499 blocks is the largest write that
// fits.
//
// The limit counts entries, not bytes. 200 blocks carrying 3.0 MB of payload
// published fine while 3860 blocks carrying 1.4 MB failed, so payload size is
// reported for diagnostics but never thresholded. Source lines are useless as a
// predictor too: a table-heavy 1322-line document produced 4620 entries while a
// text-only 1642-line document produced 820, because each table expands into
// row, cell, and text blocks.
const (
	// maxChangeEntriesPerWrite is the server-enforced ceiling on change_map
	// entries, including the page root entry.
	maxChangeEntriesPerWrite = 3500

	// maxBlockEntriesPerWrite is the matching ceiling on block entries alone.
	maxBlockEntriesPerWrite = maxChangeEntriesPerWrite - 1

	// blockEntryWriteBudget is the per-write budget used when splitting a large
	// document. It keeps a margin under the measured ceiling because the limit
	// was measured in one document shape and the validator's exact rule (entries
	// only, or entries plus ops) is not established.
	blockEntryWriteBudget = 3400
)

// changeEntryCount converts a block entry count into the change_map size the
// server actually counts, which includes the page root entry.
func changeEntryCount(blockEntries int) int {
	return blockEntries + 1
}

// sizeAdvisory describes the measured size of a planned write.
type sizeAdvisory struct {
	BlockEntries int
	PayloadBytes int
	WriteCount   int
}

// measureSpecs builds the blocks a write would produce and reports how many
// entries it contains and how many bytes their serialized payloads occupy.
//
// Block building is pure: it allocates IDs and shapes data without contacting
// the server, so placeholder page and author identifiers are enough to measure a
// write during a dry run. The measured entry count is what the server receives,
// which runs far above the spec count because each table expands into row, cell,
// and text blocks.
func measureSpecs(specs []Spec) (int, int) {
	_, entries := buildBlocks(specs, "dry-run-page", newBlockFactory("dry-run-author"))
	return len(entries), entryPayloadBytes(entries)
}

func entryPayloadBytes(entries []blockEntry) int {
	payloadBytes := 0
	for _, entry := range entries {
		if encoded, err := json.Marshal(entry.Data); err == nil {
			payloadBytes += len(encoded)
		}
	}
	return payloadBytes
}

// specSizeAdvisory measures the given specs and reports how many writes they need.
func specSizeAdvisory(specs []Spec) sizeAdvisory {
	blockEntries, payloadBytes := measureSpecs(specs)
	advisory := sizeAdvisory{BlockEntries: blockEntries, PayloadBytes: payloadBytes}
	if groups, err := partitionWriteGroups(specs); err == nil {
		advisory.WriteCount = len(groups)
	}
	return advisory
}

// applyTo attaches measured size fields to a dry-run payload.
func (a sizeAdvisory) applyTo(payload map[string]any) map[string]any {
	payload["plannedBlockEntries"] = a.BlockEntries
	payload["plannedChangeEntries"] = changeEntryCount(a.BlockEntries)
	payload["plannedPayloadBytes"] = a.PayloadBytes
	payload["maxChangeEntriesPerWrite"] = maxChangeEntriesPerWrite
	if a.WriteCount > 0 {
		payload["plannedWriteCount"] = a.WriteCount
	}
	if a.WriteCount > 1 {
		payload["willSplitWrites"] = true
		payload["splitReason"] = fmt.Sprintf(
			"planned write is %d change entries; the endpoint accepts at most %d per write, so the content is written in %d sequential writes",
			changeEntryCount(a.BlockEntries), maxChangeEntriesPerWrite, a.WriteCount)
	}
	return payload
}

// writeSizeHint explains a rejected write when its change_map exceeded the
// measured ceiling. Above the limit the cause is established, so the message
// states it; at or below the limit it says size is probably not the cause, to
// avoid misdirecting the reader.
func writeSizeHint(changeEntries int) string {
	if changeEntries > maxChangeEntriesPerWrite {
		return fmt.Sprintf(
			" — the write carried %d change_map entries and the endpoint accepts at most %d (a page root entry plus %d block entries);"+
				" this is a count limit, not a byte limit",
			changeEntries, maxChangeEntriesPerWrite, maxBlockEntriesPerWrite)
	}
	return fmt.Sprintf(
		" — the write carried %d change_map entries, within the measured limit of %d, so write size is probably not the cause",
		changeEntries, maxChangeEntriesPerWrite)
}
