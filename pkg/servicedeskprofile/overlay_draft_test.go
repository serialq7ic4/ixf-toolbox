package servicedeskprofile

import (
	"strings"
	"testing"
)

const validDraftJSON = `{
  "schemaVersion": 1,
  "name": "hardware-repair",
  "description": "",
  "routing": {
    "displayName": "",
    "useWhen": "",
    "doNotUseWhen": "",
    "examples": []
  },
  "target": {"businessId": 15, "serviceId": 4003},
  "baseline": {
    "snapshotId": "218527",
    "serviceKey": "request",
    "metadataHash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "fields": [
      {
        "stableId": "faults",
        "name": "Faults",
        "type": "extend_table",
        "required": false,
        "columns": []
      }
    ]
  },
  "fields": {
    "faults": {
      "match": {"stableId": "faults", "name": "Faults"},
      "columns": {}
    }
  }
}`

func TestDecodeOverlayDraftAllowsIncompleteRoutingAndPreservesTableColumns(t *testing.T) {
	overlay, err := DecodeOverlayDraft([]byte(validDraftJSON))
	if err != nil {
		t.Fatalf("DecodeOverlayDraft() error = %v", err)
	}
	if len(overlay.Baseline.Fields) != 1 || overlay.Baseline.Fields[0].Columns == nil {
		t.Fatalf("decoded table columns = %#v, want present empty slice", overlay.Baseline.Fields)
	}
	if err := ValidateOverlaySchema(overlay); err == nil || !strings.Contains(err.Error(), "routing") {
		t.Fatalf("ValidateOverlaySchema() error = %v, want incomplete routing", err)
	}
}

func TestDecodeOverlayDraftPreservesStrictWireRules(t *testing.T) {
	tests := map[string]string{
		"unknown field":   strings.Replace(validDraftJSON, `"schemaVersion": 1,`, `"schemaVersion": 1, "unknown": true,`, 1),
		"null optional":   strings.Replace(validDraftJSON, `"columns": {}`, `"aliases": null, "columns": {}`, 1),
		"trailing JSON":   validDraftJSON + `{}`,
		"bad fingerprint": strings.Replace(validDraftJSON, `sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`, `sha256:ABC`, 1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeOverlayDraft([]byte(raw)); err == nil {
				t.Fatal("DecodeOverlayDraft() succeeded, want strict decode failure")
			}
		})
	}
}
