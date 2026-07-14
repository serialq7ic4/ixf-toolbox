package markdown

import "testing"

func TestOutlineUsesH2BelowSingleTitleH1(t *testing.T) {
	doc := "# Title\n\n## One\n\nAlpha\n\n## Two\n\nBeta\n"

	outline, err := BuildOutline(doc, 100)
	if err != nil {
		t.Fatalf("BuildOutline returned error: %v", err)
	}

	if outline.SelectedHeadingLevel == nil || *outline.SelectedHeadingLevel != 2 {
		t.Fatalf("selected heading level = %v, want 2", outline.SelectedHeadingLevel)
	}
	got := []string{outline.Chunks[len(outline.Chunks)-2].Breadcrumb, outline.Chunks[len(outline.Chunks)-1].Breadcrumb}
	want := []string{"Title > One", "Title > Two"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("breadcrumb[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOutlineKeepsCodeTablesAndImagesAtomic(t *testing.T) {
	doc := "# Title\n\n## Data\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n![Diagram](assets/docx_1/image-001.png)\n*Architecture caption*\n\n```python\n# fake heading\nprint('x')\n```\n"

	outline, err := BuildOutline(doc, 20)
	if err != nil {
		t.Fatalf("BuildOutline returned error: %v", err)
	}

	rendered := make([]string, 0, len(outline.Chunks))
	images := []string{}
	for _, chunk := range outline.Chunks {
		text, err := RenderChunk(doc, outline, chunk.Index)
		if err != nil {
			t.Fatalf("RenderChunk(%d) returned error: %v", chunk.Index, err)
		}
		rendered = append(rendered, text)
		images = append(images, chunk.ImagePaths...)
	}

	assertContainsChunk(t, rendered, "| A | B |\n| --- | --- |\n| 1 | 2 |")
	assertContainsChunk(t, rendered, "![Diagram](assets/docx_1/image-001.png)\n*Architecture caption*")
	assertContainsChunk(t, rendered, "```python\n# fake heading\nprint('x')\n```\n")
	if len(images) != 1 || images[0] != "assets/docx_1/image-001.png" {
		t.Fatalf("image paths = %#v, want assets/docx_1/image-001.png", images)
	}
}

func TestRenderChunkRejectsOutOfRangeIndex(t *testing.T) {
	doc := "# Title\n\nBody\n"
	outline, err := BuildOutline(doc, 100)
	if err != nil {
		t.Fatalf("BuildOutline returned error: %v", err)
	}

	_, err = RenderChunk(doc, outline, 2)
	if err == nil {
		t.Fatal("RenderChunk accepted an out-of-range index")
	}
}

func assertContainsChunk(t *testing.T, chunks []string, needle string) {
	t.Helper()
	for _, chunk := range chunks {
		if stringsContains(chunk, needle) {
			return
		}
	}
	t.Fatalf("no chunk contained %q in %#v", needle, chunks)
}

func stringsContains(haystack string, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && containsAt(haystack, needle))
}

func containsAt(haystack string, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
