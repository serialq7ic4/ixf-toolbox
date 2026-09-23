package okr

import "testing"

func inspectFixtureState() []objectiveState {
	return []objectiveState{
		{
			ID:        "o1",
			Objective: "支撑计算平台规模化落地",
			KRs: []krState{
				{ID: "kr1", Text: "SAE 生产应用数提升到 8000"},
				{ID: "kr2", Text: "KubeVirt 云主机提升到 500+"},
			},
		},
		{ID: "o2", Objective: "提升交付效率", KRs: []krState{}},
	}
}

func TestInspectPayloadReportsIndexesAndCounts(t *testing.T) {
	payload := inspectPayload("okr-fixture", inspectFixtureState())

	if payload["okrId"] != "okr-fixture" {
		t.Fatalf("okrId = %v", payload["okrId"])
	}
	if payload["objectiveCount"] != 2 {
		t.Fatalf("objectiveCount = %v, want 2", payload["objectiveCount"])
	}
	if payload["krCount"] != 2 {
		t.Fatalf("krCount = %v, want 2 across all objectives", payload["krCount"])
	}
	objectives := payload["objectives"].([]map[string]any)
	if len(objectives) != 2 {
		t.Fatalf("objectives length = %d", len(objectives))
	}
	if objectives[0]["index"] != 1 || objectives[1]["index"] != 2 {
		t.Fatalf("indexes should be 1-based to match --objective-index: %+v", objectives)
	}
	if objectives[0]["id"] != "o1" || objectives[0]["krCount"] != 2 {
		t.Fatalf("objective 1 = %+v", objectives[0])
	}
}

// The index one past the end creates rather than replaces, and which one a caller
// wants depends on the current count, so the payload states it directly.
func TestInspectPayloadReportsNextObjectiveIndex(t *testing.T) {
	payload := inspectPayload("okr-fixture", inspectFixtureState())
	if payload["nextObjectiveIndex"] != 3 {
		t.Fatalf("nextObjectiveIndex = %v, want 3 for two existing objectives", payload["nextObjectiveIndex"])
	}

	empty := inspectPayload("okr-fixture", nil)
	if empty["nextObjectiveIndex"] != 1 {
		t.Fatalf("nextObjectiveIndex = %v, want 1 for an empty page", empty["nextObjectiveIndex"])
	}
	if empty["objectiveCount"] != 0 {
		t.Fatalf("objectiveCount = %v, want 0", empty["objectiveCount"])
	}
}

// KR identifiers are what a caller needs to reason about replacement, so they must
// be reported rather than only counted.
func TestInspectPayloadReportsKRIdentifiersAndOrder(t *testing.T) {
	payload := inspectPayload("okr-fixture", inspectFixtureState())
	objectives := payload["objectives"].([]map[string]any)
	krs := objectives[0]["krs"].([]map[string]any)
	if len(krs) != 2 {
		t.Fatalf("krs length = %d, want 2", len(krs))
	}
	if krs[0]["index"] != 1 || krs[0]["id"] != "kr1" || krs[0]["text"] != "SAE 生产应用数提升到 8000" {
		t.Fatalf("kr 1 = %+v", krs[0])
	}
	if krs[1]["index"] != 2 || krs[1]["id"] != "kr2" {
		t.Fatalf("kr 2 = %+v", krs[1])
	}
}

func TestInspectPayloadReportsRemainingCapacity(t *testing.T) {
	payload := inspectPayload("okr-fixture", inspectFixtureState())
	objectives := payload["objectives"].([]map[string]any)
	if objectives[0]["krRemaining"] != maxKRsPerObjective-2 {
		t.Fatalf("objective 1 krRemaining = %v", objectives[0]["krRemaining"])
	}
	if objectives[1]["krRemaining"] != maxKRsPerObjective {
		t.Fatalf("objective 2 krRemaining = %v, want the full ceiling", objectives[1]["krRemaining"])
	}
}

// An objective already at or over the ceiling must report zero remaining rather
// than a negative number.
func TestInspectPayloadClampsRemainingCapacityAtZero(t *testing.T) {
	krs := make([]krState, maxKRsPerObjective+2)
	for index := range krs {
		krs[index] = krState{ID: "kr", Text: "text"}
	}
	payload := inspectPayload("okr-fixture", []objectiveState{{ID: "o1", Objective: "O", KRs: krs}})
	objectives := payload["objectives"].([]map[string]any)
	if objectives[0]["krRemaining"] != 0 {
		t.Fatalf("krRemaining = %v, want 0 when over the ceiling", objectives[0]["krRemaining"])
	}
}

func TestInspectRejectsNonOKRSource(t *testing.T) {
	if _, err := Inspect(ReadConfig{Source: "https://example.test/docx/abc"}); err == nil {
		t.Fatal("expected a non-OKR URL to be rejected")
	}
}
