package docxgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type Signature struct {
	value string
}

func (s Signature) Equal(other Signature) bool {
	return s.value == other.value
}

func (g Graph) RootSignature() Signature {
	builder := strings.Builder{}
	for _, blockID := range g.RootChildren {
		g.writeBlockSignature(&builder, blockID, map[string]bool{})
	}
	return hashSignature(builder.String())
}

func FingerprintText(text string) string {
	return hashString(normalizeFingerprintText(text))
}

func (g Graph) SectionFingerprint(r SectionRange) string {
	builder := strings.Builder{}
	for _, blockID := range r.IDs {
		text := normalizeFingerprintText(g.blockSubtreeText(blockID, map[string]bool{}))
		if text == "" {
			continue
		}
		builder.WriteString(text)
		builder.WriteString("\n")
	}
	return hashString(builder.String())
}

func (g Graph) SectionContainsFingerprint(r SectionRange, fingerprint string) bool {
	if fingerprint == "" {
		return false
	}
	if g.SectionFingerprint(r) == fingerprint {
		return true
	}
	for _, blockID := range r.IDs {
		if FingerprintText(g.blockSubtreeText(blockID, map[string]bool{})) == fingerprint {
			return true
		}
	}
	return false
}

func (g Graph) writeBlockSignature(builder *strings.Builder, blockID string, seen map[string]bool) {
	if blockID == "" || seen[blockID] {
		return
	}
	seen[blockID] = true
	block := g.Blocks[blockID]
	builder.WriteString(block.ID)
	builder.WriteString("|")
	builder.WriteString(block.Kind)
	builder.WriteString("|")
	builder.WriteString(NormalizeHeading(block.Text))
	builder.WriteString("|")
	for _, childID := range block.Children {
		builder.WriteString(childID)
		builder.WriteString(",")
	}
	builder.WriteString("\n")
	for _, childID := range block.Children {
		g.writeBlockSignature(builder, childID, seen)
	}
}

func (g Graph) blockSubtreeText(blockID string, seen map[string]bool) string {
	if blockID == "" || seen[blockID] {
		return ""
	}
	seen[blockID] = true
	block := g.Blocks[blockID]
	parts := []string{}
	if strings.TrimSpace(block.Text) != "" {
		parts = append(parts, block.Text)
	}
	for _, childID := range block.Children {
		if childText := g.blockSubtreeText(childID, seen); strings.TrimSpace(childText) != "" {
			parts = append(parts, childText)
		}
	}
	return strings.Join(parts, "\n")
}

func hashSignature(value string) Signature {
	return Signature{value: hashString(value)}
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func normalizeFingerprintText(text string) string {
	return NormalizeHeading(strings.ReplaceAll(text, "\r\n", "\n"))
}
