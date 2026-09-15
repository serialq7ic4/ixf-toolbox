package docspublish

import (
	"strings"
	"testing"
)

func TestSpecTreeCountsIncludeChildren(t *testing.T) {
	specs := []Spec{{Kind: "ordered", Text: "Step", Children: []Spec{
		{Kind: "code", Text: "echo one"},
		{Kind: "image", SourceKind: "mermaid", Text: "flowchart LR\nA-->B"},
	}}}
	counts := summarizeSpecs(specs)
	if counts["ordered"] != 1 || counts["code"] != 1 || counts["image"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
	if countSpecsBySourceKind(specs, "image", "mermaid") != 1 || countSpecsByKind(specs, "image") != 1 {
		t.Fatalf("recursive image counts are missing: %#v", counts)
	}
	if countNestedSpecs(specs) != 2 {
		t.Fatalf("nested count = %d, want 2", countNestedSpecs(specs))
	}
}

func TestSpecTreeRejectsChildrenOnUnsupportedParent(t *testing.T) {
	err := validateSpecTree([]Spec{{Kind: "text", Text: "body", Children: []Spec{{Kind: "code", Text: "bad"}}}})
	if err == nil || !strings.Contains(err.Error(), "spec kind \"text\" cannot contain children") {
		t.Fatalf("err = %v, want unsupported parent error", err)
	}
}

func TestSpecTreeFingerprintAndRequiredTextIncludeCodeChildren(t *testing.T) {
	specs := []Spec{{Kind: "ordered", Text: "Step", Children: []Spec{{Kind: "code", Text: "unique-command"}}}}
	if fingerprintSpecs(specs) == fingerprintSpecs([]Spec{{Kind: "ordered", Text: "Step"}}) {
		t.Fatal("fingerprint ignored child code text")
	}
	required := patchVerifyRequiredText(specs, nil)
	if !strings.Contains(strings.Join(required, "\n"), "unique-command") {
		t.Fatalf("required text = %#v, want child code text", required)
	}
}

