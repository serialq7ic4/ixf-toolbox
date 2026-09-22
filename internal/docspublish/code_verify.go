package docspublish

import "strings"

// missingCodeBlockTexts reports the code block bodies that the source specs expect
// but the written document does not contain.
//
// This replaces an earlier heuristic that required at least one code block in the
// document to contain a newline. The intent was to catch a multi-line code block
// arriving flattened, because multi-line text uses a different attribute encoding
// than single-line text (see attribForTextAttrs). But presence of a newline
// somewhere is not that check: a document whose code blocks are all legitimately
// single-line satisfied nothing and was reported as unverified, while a single
// multi-line block masked every other block being flattened.
//
// Comparing recovered text against the source text checks what the heuristic was
// reaching for, and it holds for single-line and multi-line blocks alike.
func missingCodeBlockTexts(specs []Spec, recovered []string) []string {
	expected := expectedCodeTexts(specs)
	if len(expected) == 0 {
		return nil
	}
	remaining := make([]string, len(recovered))
	copy(remaining, recovered)

	missing := []string{}
	for _, want := range expected {
		matched := false
		for index, have := range remaining {
			if codeTextMatches(want, have) {
				remaining = append(remaining[:index], remaining[index+1:]...)
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, want)
		}
	}
	return missing
}

// expectedCodeTexts collects the non-empty code block bodies from the spec tree,
// including code blocks nested as children of ordered list items.
func expectedCodeTexts(specs []Spec) []string {
	texts := []string{}
	walkSpecs(specs, func(spec Spec) {
		if spec.Kind != "code" {
			return
		}
		if normalizeCodeText(spec.Text) == "" {
			return
		}
		texts = append(texts, spec.Text)
	})
	return texts
}

// codeTextMatches compares a source code body against a recovered one.
//
// The comparison is exact once line endings and trailing whitespace are
// normalized, so a flattened multi-line block does not match its source: losing
// the newlines changes the normalized text.
func codeTextMatches(want, have string) bool {
	return normalizeCodeText(want) == normalizeCodeText(have)
}

func normalizeCodeText(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
