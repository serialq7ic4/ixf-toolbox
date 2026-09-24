package okr

import (
	"strings"
	"testing"
)

// Every test here covers a shape the live server actually sends and the fixtures did
// not. Each of these defects was found by inspecting a real page, not by the suite.

// The server sends rich text either as a structure or as a string holding that same
// structure serialized. The string form was returned verbatim, so an objective title
// came back as raw delta-doc JSON.
func TestItemTextDecodesSerializedRichText(t *testing.T) {
	serialized := `{"0":{"ops":[{"insert":"微CMDB 建设","attributes":{"fontfamily":"Times"}}]}}`
	if got := itemText(map[string]any{"content": serialized}); got != "微CMDB 建设" {
		t.Fatalf("itemText = %q, want the extracted text", got)
	}
}

func TestItemTextDecodesSerializedRichTextWithMultipleOps(t *testing.T) {
	serialized := `{"0":{"ops":[{"insert":"集群部署\n"},{"insert":"编写部署流程文档"}]}}`
	if got := itemText(map[string]any{"name": serialized}); got != "集群部署\n编写部署流程文档" {
		t.Fatalf("itemText = %q", got)
	}
}

// Plain text must pass through untouched: a sentence parses as neither a JSON object
// nor an array, so the retry must not alter ordinary values.
func TestItemTextLeavesPlainTextAlone(t *testing.T) {
	if got := itemText(map[string]any{"name": "Improve delivery reliability"}); got != "Improve delivery reliability" {
		t.Fatalf("itemText = %q, want the plain string", got)
	}
}

// A string that merely starts with a brace but is not valid JSON must be kept.
func TestItemTextKeepsBraceTextThatIsNotJSON(t *testing.T) {
	if got := itemText(map[string]any{"name": "{not json at all"}); got != "{not json at all" {
		t.Fatalf("itemText = %q, want the original string", got)
	}
}

func TestItemTextStillHandlesDecodedRichText(t *testing.T) {
	decoded := map[string]any{"blocks": []any{map[string]any{"text": "Decoded title"}}}
	if got := itemText(map[string]any{"name": decoded}); got != "Decoded title" {
		t.Fatalf("itemText = %q", got)
	}
}

// The live endpoint sends okr_draft_version as a JSON number. Read with a
// string-only accessor it yielded "", so every write failed with "unable to
// determine the OKR draft version" while the endpoint was answering correctly.
func TestVersionValueReadsANumericVersion(t *testing.T) {
	source := map[string]any{"okr_draft_version": float64(1783602170161)}
	if got := versionValue(source, "okr_draft_version"); got != "1783602170161" {
		t.Fatalf("versionValue = %q, want the integer rendering", got)
	}
}

// A millisecond timestamp must not be rendered in exponent form, which is what the
// default float formatting would produce.
func TestVersionValueAvoidsExponentNotation(t *testing.T) {
	got := versionValue(map[string]any{"version": float64(1783602170161)}, "version")
	for _, bad := range []string{"e+", "E+", "."} {
		if strings.Contains(got, bad) {
			t.Fatalf("versionValue = %q, must be a plain integer", got)
		}
	}
}

func TestVersionValueStillReadsAStringVersion(t *testing.T) {
	if got := versionValue(map[string]any{"draft_version": "7"}, "draft_version"); got != "7" {
		t.Fatalf("versionValue = %q, want 7", got)
	}
}

func TestVersionValuePrefersTheFirstPresentKey(t *testing.T) {
	source := map[string]any{"draft_version": "second", "okr_draft_version": "first"}
	if got := versionValue(source, "okr_draft_version", "draft_version"); got != "first" {
		t.Fatalf("versionValue = %q, want the first key given", got)
	}
}

func TestVersionValueSkipsEmptyAndMissingKeys(t *testing.T) {
	source := map[string]any{"okr_draft_version": "   ", "draft_version": float64(42)}
	if got := versionValue(source, "okr_draft_version", "draft_version"); got != "42" {
		t.Fatalf("versionValue = %q, want the next usable key", got)
	}
	if got := versionValue(map[string]any{}, "absent"); got != "" {
		t.Fatalf("versionValue = %q, want empty for a missing key", got)
	}
}

// draftVersionFromPayload had the same string-only defect as currentDraftVersion.
// A write response carries the advanced draft version; dropping it left the cache
// holding the pre-write value, so the next call in a multi-step write failed as
// stale. Found only after fixing its sibling, because until then no write got far
// enough to reach a second call.
func TestDraftVersionFromPayloadReadsANumericVersion(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"draft_version": float64(1790218046054)}}
	if got := draftVersionFromPayload(payload); got != "1790218046054" {
		t.Fatalf("draftVersionFromPayload = %q, want the integer rendering", got)
	}
}

func TestDraftVersionFromPayloadReadsATopLevelNumericVersion(t *testing.T) {
	payload := map[string]any{"okr_draft_version": float64(42)}
	if got := draftVersionFromPayload(payload); got != "42" {
		t.Fatalf("draftVersionFromPayload = %q, want 42", got)
	}
}

func TestDraftVersionFromPayloadStillReadsAString(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"draft_version": "7"}}
	if got := draftVersionFromPayload(payload); got != "7" {
		t.Fatalf("draftVersionFromPayload = %q, want 7", got)
	}
}

func TestDraftVersionFromPayloadIsEmptyWhenAbsent(t *testing.T) {
	if got := draftVersionFromPayload(map[string]any{"data": map[string]any{}}); got != "" {
		t.Fatalf("draftVersionFromPayload = %q, want empty", got)
	}
}
