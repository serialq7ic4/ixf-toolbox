package docslocal

import (
	"strings"
	"testing"
)

// glyph builds one text-layer entry. y is in PDF page space, growing upward.
func glyph(text string, x, y, width, height float64) map[string]any {
	return map[string]any{"t": text, "r": []any{x, y, width, height}, "f": float64(1), "h": height}
}

// Kangxi radicals are what PDF extraction emits for some CJK fonts. They look like
// 子 and 支 but are different code points, so a reader or a model sees the wrong
// character and search misses it.
func TestNormalizeBoxTextFoldsKangxiRadicals(t *testing.T) {
	got := normalizeBoxText("⼦Agent与Skill，条件分⽀路由")
	want := "子Agent与Skill，条件分支路由"
	if got != want {
		t.Fatalf("normalizeBoxText = %q, want %q", got, want)
	}
}

// Full NFKC would fold the fullwidth comma to an ASCII one, rewriting ordinary
// Chinese typography. Only the radical block is an extraction artefact.
func TestNormalizeBoxTextKeepsFullwidthPunctuation(t *testing.T) {
	for _, text := range []string{"工具注册、发现", "案例：回放平台", "主从，对等"} {
		if got := normalizeBoxText(text); got != text {
			t.Fatalf("normalizeBoxText(%q) = %q, want it unchanged", text, got)
		}
	}
}

// y grows upward, so the line with the largest y is read first.
func TestLayoutBoxPageReadsTopToBottom(t *testing.T) {
	items := parseBoxTextItems([]any{
		glyph("bottom", 10, 60, 30, 4),
		glyph("top", 10, 230, 30, 4),
		glyph("middle", 10, 140, 30, 4),
	})
	lines := layoutBoxPage(items, 4)
	got := []string{}
	for _, line := range lines {
		got = append(got, line.text)
	}
	if strings.Join(got, "|") != "top|middle|bottom" {
		t.Fatalf("lines = %v, want top, middle, bottom", got)
	}
}

// A watermark crossing body text is fused into the glyph it overlaps, arriving as
// "h\n二期". The glyph's box is one line tall, so the text after the break is not
// part of it.
func TestLayoutBoxPageDropsOverlayFusedIntoAGlyph(t *testing.T) {
	items := parseBoxTextItems([]any{
		glyph("A", 10, 100, 4, 4),
		glyph("r", 14, 100, 4, 4),
		glyph("c\r\n二", 18, 100, 4, 4),
		glyph("h\r\n二期 ", 22, 100, 4, 4),
	})
	lines := layoutBoxPage(items, 4)
	if len(lines) != 1 || lines[0].text != "Arch" {
		t.Fatalf("lines = %#v, want the single line Arch", lines)
	}
}

// Kerning is a fraction of a glyph and must not become a space; a column gutter is
// many glyphs wide and must. A quarter-glyph threshold split "Course".
func TestJoinBoxRunsSpacesOnlyAtColumnGaps(t *testing.T) {
	items := parseBoxTextItems([]any{
		glyph("C", 0, 100, 3, 4), glyph("o", 3.5, 100, 3, 4), glyph("u", 7, 100, 3, 4),
		glyph("r", 11.5, 100, 3, 4), glyph("s", 15, 100, 3, 4), glyph("e", 18.5, 100, 3, 4),
		glyph("右", 80, 100, 4, 4), glyph("栏", 84, 100, 4, 4),
	})
	lines := layoutBoxPage(items, 4)
	if len(lines) != 1 || lines[0].text != "Course 右栏" {
		t.Fatalf("lines = %#v, want one line \"Course 右栏\"", lines)
	}
}

// The same glyph drawn twice at one position, as bold text sometimes is, is kept once.
func TestJoinBoxRunsDropsAGlyphRepeatedInPlace(t *testing.T) {
	items := parseBoxTextItems([]any{
		glyph("P", 0, 100, 3, 4), glyph("P", 0.4, 100, 3, 4),
		glyph("A", 3.5, 100, 3, 4), glyph("R", 7, 100, 3, 4), glyph("T", 10.5, 100, 3, 4),
	})
	lines := layoutBoxPage(items, 4)
	if len(lines) != 1 || lines[0].text != "PART" {
		t.Fatalf("lines = %#v, want PART", lines)
	}
}

// Page keys are decimal strings. Ordered as strings, "10" comes before "2".
func TestRenderBoxTextLayerOrdersPagesNumerically(t *testing.T) {
	page := map[string]any{}
	for _, key := range []string{"0", "1", "2", "10", "11"} {
		page[key] = []any{glyph("p"+key, 10, 100, 12, 4)}
	}
	lines, pages := renderBoxTextLayer(map[string]any{"page": page})
	if pages != 5 {
		t.Fatalf("pages = %d, want 5", pages)
	}
	order := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, "p") {
			order = append(order, line)
		}
	}
	if strings.Join(order, ",") != "p0,p1,p2,p10,p11" {
		t.Fatalf("page order = %v, want numeric", order)
	}
}

// A tiled watermark is reported as tall boxes of one identical size on every page.
// Real text blocks vary in size, so they survive while the tiles go.
//
// Each page carries a run of body glyphs, as a real page does. The block threshold
// is relative to the typical glyph height, which is a body glyph's only when body
// glyphs are the majority; a page of one glyph and two tall boxes is not a shape a
// text document produces.
func TestRenderBoxTextLayerDropsUniformTallTiles(t *testing.T) {
	page := map[string]any{}
	for index, key := range []string{"0", "1", "2", "3", "4"} {
		entries := []any{
			glyph("WATERMARK", 50, 50, 118.1, 125.3),
			// A genuine block: sized and worded differently on each page, as real
			// content is. Identical text on every page would be a running header,
			// which a separate filter is right to drop.
			glyph("block "+key+" one\r\nblock "+key+" two", 10, 150, 60, 30+float64(index)),
		}
		for offset, char := range "body" + key + "text" {
			entries = append(entries, glyph(string(char), 10+float64(offset)*4, 200, 3.5, 4))
		}
		page[key] = entries
	}
	lines, _ := renderBoxTextLayer(map[string]any{"page": page})
	text := strings.Join(lines, "\n")
	if strings.Contains(text, "WATERMARK") {
		t.Fatalf("watermark tile survived:\n%s", text)
	}
	if !strings.Contains(text, "block 3 two") {
		t.Fatalf("a real multi-line block was dropped:\n%s", text)
	}
	if !strings.Contains(text, "body3text") {
		t.Fatalf("body text was dropped:\n%s", text)
	}
}

// A line recurring verbatim on most pages is a running header and is dropped.
func TestRenderBoxTextLayerDropsRepeatedHeaders(t *testing.T) {
	page := map[string]any{}
	for _, key := range []string{"0", "1", "2", "3"} {
		page[key] = []any{
			glyph("Course Header", 10, 280, 60, 4),
			glyph("content "+key, 10, 100, 40, 4),
		}
	}
	lines, _ := renderBoxTextLayer(map[string]any{"page": page})
	text := strings.Join(lines, "\n")
	if strings.Contains(text, "Course Header") {
		t.Fatalf("repeated header survived:\n%s", text)
	}
	if !strings.Contains(text, "content 2") {
		t.Fatalf("page content was dropped:\n%s", text)
	}
}

// A short document cannot distinguish a header from coincidence, so it keeps all.
func TestRenderBoxTextLayerKeepsLinesInShortDocuments(t *testing.T) {
	page := map[string]any{
		"0": []any{glyph("Same line", 10, 100, 40, 4)},
		"1": []any{glyph("Same line", 10, 100, 40, 4)},
	}
	lines, _ := renderBoxTextLayer(map[string]any{"page": page})
	if strings.Count(strings.Join(lines, "\n"), "Same line") != 2 {
		t.Fatalf("a two-page document lost a line: %v", lines)
	}
}

// Leading "#" would compete with the page headings; a fence would swallow the rest.
func TestEscapeBoxMarkdownLine(t *testing.T) {
	cases := map[string]string{
		"# not a heading": `\# not a heading`,
		"```go":           "\\```go",
		"- a list item":   "- a list item",
		"plain text":      "plain text",
	}
	for input, want := range cases {
		if got := escapeBoxMarkdownLine(input); got != want {
			t.Fatalf("escapeBoxMarkdownLine(%q) = %q, want %q", input, got, want)
		}
	}
}

// Where the watermark crosses empty space its glyphs stand alone and form lines of
// their own. Those lines are pieces of the watermark, recovered from the tiles, and
// go; a line of body text that merely shares a character with it stays.
func TestRenderBoxTextLayerDropsStandaloneWatermarkFragments(t *testing.T) {
	page := map[string]any{}
	for _, key := range []string{"0", "1", "2", "3", "4"} {
		entries := []any{glyph("TRAINING CAMP", 50, 50, 118.1, 125.3)}
		for offset, char := range "real" + key {
			entries = append(entries, glyph(string(char), 10+float64(offset)*4, 200, 3.5, 4))
		}
		page[key] = entries
	}
	// A fragment of the watermark standing alone on one page.
	page["2"] = append(page["2"].([]any), glyph("CAMP", 10, 120, 16, 4))
	// Body text sharing letters with the watermark, which must survive.
	page["3"] = append(page["3"].([]any), glyph("CAMPAIGN", 10, 120, 32, 4))
	lines, _ := renderBoxTextLayer(map[string]any{"page": page})
	text := strings.Join(lines, "\n")
	if strings.Contains(text, "\nCAMP\n") {
		t.Fatalf("a standalone watermark fragment survived:\n%s", text)
	}
	if !strings.Contains(text, "CAMPAIGN") {
		t.Fatalf("body text sharing letters with the watermark was dropped:\n%s", text)
	}
}
