package docslocal

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// boxTextItem is one entry of a FAST_PDF text layer.
//
// The layer is produced by the server's preview conversion and served as
// txt_all.json. Most items are single glyphs, but some are whole text blocks whose
// text carries its own CRLF line breaks and whose bounding box spans several lines.
// r is [x, y, width, height] in PDF page space: the origin is the bottom-left
// corner and y grows upward, so the top of the page has the largest y and reading
// order is descending y. On a real slide the title sat at y=232 of a 303-point
// page and the last row at y=64.
type boxTextItem struct {
	text   string
	x      float64
	y      float64
	width  float64
	height float64
}

// boxTextLine is one reconstructed line of a page, positioned for ordering.
type boxTextLine struct {
	y    float64
	x    float64
	text string
}

// boxRepeatedLineShare is the fraction of pages a line must appear on, unchanged,
// to be treated as a running header or footer and dropped.
//
// Slide-derived PDFs repeat a course or brand name on every page. Leaving it in
// puts the same line in front of every page's content, which is noise for a reader
// and dilutes what a model sees. Half the pages is a deliberately conservative bar:
// genuine content rarely repeats verbatim that often, and a document too short to
// show a pattern is left alone entirely.
const boxRepeatedLineShare = 0.5

// boxRepeatedLineMinPages is the page count below which no line is treated as
// repeated, since two or three pages cannot distinguish a header from coincidence.
const boxRepeatedLineMinPages = 4

// parseBoxTextItems reads one page's items from the decoded text layer.
func parseBoxTextItems(raw []any) []boxTextItem {
	items := make([]boxTextItem, 0, len(raw))
	for _, entry := range raw {
		item := asMap(entry)
		rect := asSlice(item["r"])
		if len(rect) < 4 {
			continue
		}
		text := normalizeBoxText(stringValue(item["t"]))
		if strings.TrimSpace(text) == "" {
			continue
		}
		items = append(items, boxTextItem{
			text:   text,
			x:      floatOrZero(rect[0]),
			y:      floatOrZero(rect[1]),
			width:  floatOrZero(rect[2]),
			height: floatOrZero(rect[3]),
		})
	}
	return items
}

func floatOrZero(value any) float64 {
	if number, ok := floatValue(value); ok {
		return number
	}
	return 0
}

// kangxiRadicalSources and kangxiRadicalTargets map the Kangxi Radicals and CJK
// Radicals Supplement code points that have a compatibility decomposition to the
// unified ideograph they decompose to. Both strings are indexed by rune.
//
// PDF text extraction emits these for Chinese text set in some CJK fonts: the glyph
// looks identical to 子 or 支, but the code point is U+2F26 or U+2F40, so a reader
// or a model sees a different character and search misses it. They are generated
// from Unicode's NFKC table rather than typed, so they are exactly the characters
// whose decomposition is a single ideograph.
//
// Full NFKC is deliberately not applied. It would also fold fullwidth punctuation
// such as ， and ： to ASCII, rewriting ordinary Chinese typography that was correct
// to begin with. Only this block is a known extraction artefact, so only it is
// rewritten.
const (
	kangxiRadicalSources = "⺟⻳⼀⼁⼂⼃⼄⼅⼆⼇⼈⼉⼊⼋⼌⼍⼎⼏⼐⼑⼒⼓⼔⼕⼖⼗⼘⼙⼚⼛⼜⼝⼞⼟⼠⼡⼢⼣⼤⼥⼦⼧⼨⼩⼪⼫⼬⼭⼮⼯⼰⼱⼲⼳⼴⼵⼶⼷⼸⼹⼺⼻⼼⼽⼾⼿⽀⽁⽂⽃⽄⽅⽆⽇⽈⽉⽊⽋⽌⽍⽎⽏⽐⽑⽒⽓⽔⽕⽖⽗⽘⽙⽚⽛⽜⽝⽞⽟⽠⽡⽢⽣⽤⽥⽦⽧⽨⽩⽪⽫⽬⽭⽮⽯⽰⽱⽲⽳⽴⽵⽶⽷⽸⽹⽺⽻⽼⽽⽾⽿⾀⾁⾂⾃⾄⾅⾆⾇⾈⾉⾊⾋⾌⾍⾎⾏⾐⾑⾒⾓⾔⾕⾖⾗⾘⾙⾚⾛⾜⾝⾞⾟⾠⾡⾢⾣⾤⾥⾦⾧⾨⾩⾪⾫⾬⾭⾮⾯⾰⾱⾲⾳⾴⾵⾶⾷⾸⾹⾺⾻⾼⾽⾾⾿⿀⿁⿂⿃⿄⿅⿆⿇⿈⿉⿊⿋⿌⿍⿎⿏⿐⿑⿒⿓⿔⿕"
	kangxiRadicalTargets = "母龟一丨丶丿乙亅二亠人儿入八冂冖冫几凵刀力勹匕匚匸十卜卩厂厶又口囗土士夂夊夕大女子宀寸小尢尸屮山巛工己巾干幺广廴廾弋弓彐彡彳心戈戶手支攴文斗斤方无日曰月木欠止歹殳毋比毛氏气水火爪父爻爿片牙牛犬玄玉瓜瓦甘生用田疋疒癶白皮皿目矛矢石示禸禾穴立竹米糸缶网羊羽老而耒耳聿肉臣自至臼舌舛舟艮色艸虍虫血行衣襾見角言谷豆豕豸貝赤走足身車辛辰辵邑酉釆里金長門阜隶隹雨靑非面革韋韭音頁風飛食首香馬骨高髟鬥鬯鬲鬼魚鳥鹵鹿麥麻黃黍黑黹黽鼎鼓鼠鼻齊齒龍龜龠"
)

var kangxiRadicalReplacer = func() *strings.Replacer {
	sources := []rune(kangxiRadicalSources)
	targets := []rune(kangxiRadicalTargets)
	pairs := make([]string, 0, 2*len(sources))
	for index := range sources {
		pairs = append(pairs, string(sources[index]), string(targets[index]))
	}
	return strings.NewReplacer(pairs...)
}()

// normalizeBoxText folds radical code points and unifies line breaks.
func normalizeBoxText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return kangxiRadicalReplacer.Replace(text)
}

// boxBlockHeightRatio separates multi-line text blocks from inline glyphs.
//
// An inline glyph's box is about one line tall. A text block's box spans many lines
// and carries its own line breaks, so it is laid out from its own text rather than
// merged with the glyphs beside it. A box several times the page's typical glyph
// height can only be a block.
const boxBlockHeightRatio = 4.0

// layoutBoxPage reconstructs one page's lines in reading order.
//
// referenceHeight is the typical glyph height across the whole document, not this
// page. A slide page can be made mostly of text blocks, and a per-page median taken
// over them is inflated enough that blocks stop looking like blocks.
func layoutBoxPage(items []boxTextItem, referenceHeight float64) []boxTextLine {
	if len(items) == 0 {
		return nil
	}
	lines := []boxTextLine{}
	inline := []boxTextItem{}
	for _, item := range items {
		if referenceHeight > 0 && item.height > boxBlockHeightRatio*referenceHeight {
			for offset, text := range strings.Split(item.text, "\n") {
				if trimmed := strings.TrimSpace(text); trimmed != "" {
					// Lines of one block keep their order by stepping just below the
					// block's position, so a block never interleaves with itself under
					// the descending sort.
					lines = append(lines, boxTextLine{y: item.y - float64(offset)*0.001, x: item.x, text: trimmed})
				}
			}
			continue
		}
		// An inline item's box is one line tall, so its text cannot genuinely span
		// lines. A line break inside one is the extractor fusing whatever overlapped
		// the glyph into the same item -- in practice a diagonal watermark crossing
		// the body text, which arrives as "h\n二期" on the glyph for h. On a real
		// slide deck 16% of inline items carried such a break, and in 92% of them the
		// part before it was a single body glyph while the part after was the
		// watermark. Keeping the first line removes the overlay without needing to
		// know what the watermark says.
		if head, _, found := strings.Cut(item.text, "\n"); found {
			if strings.TrimSpace(head) == "" {
				continue
			}
			item.text = head
		}
		inline = append(inline, item)
	}
	// Grouping tolerance comes from the inline glyphs alone, once blocks are out.
	lines = append(lines, groupBoxInlineLines(inline, medianBoxHeight(inline))...)
	// Descending y is top to bottom; ties read left to right.
	sort.SliceStable(lines, func(left, right int) bool {
		if lines[left].y != lines[right].y {
			return lines[left].y > lines[right].y
		}
		return lines[left].x < lines[right].x
	})
	return lines
}

// groupBoxInlineLines joins glyphs that share a baseline into lines.
func groupBoxInlineLines(items []boxTextItem, inlineHeight float64) []boxTextLine {
	if len(items) == 0 {
		return nil
	}
	sorted := append([]boxTextItem(nil), items...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return boxCenter(sorted[left]) < boxCenter(sorted[right])
	})
	tolerance := 0.5 * inlineHeight
	groups := [][]boxTextItem{}
	for _, item := range sorted {
		last := len(groups) - 1
		if last >= 0 && math.Abs(boxCenter(item)-groupCenter(groups[last])) <= tolerance {
			groups[last] = append(groups[last], item)
			continue
		}
		groups = append(groups, []boxTextItem{item})
	}
	lines := make([]boxTextLine, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group, func(left, right int) bool { return group[left].x < group[right].x })
		if text := joinBoxRuns(group); text != "" {
			lines = append(lines, boxTextLine{y: groupCenter(group), x: group[0].x, text: text})
		}
	}
	return lines
}

// boxColumnGapRatio is the horizontal gap, in glyph heights, that marks a column
// break rather than ordinary spacing.
//
// Word spaces need no inference: this layer's Latin glyphs carry their own
// trailing space. What remains is kerning, which is a fraction of a glyph, and the
// gutter between columns, which is many. A full glyph height separates the two
// cleanly; a quarter of one split "Course" into "Cou rse".
const boxColumnGapRatio = 1.0

// joinBoxRuns concatenates one line's glyphs, adding a space only at a column gap.
//
// It also drops a glyph that repeats its predecessor at nearly the same position.
// Bold text is drawn as two overlapping passes of the same glyphs, which reads as
// "PAPART" for "PART" when both passes are kept.
func joinBoxRuns(group []boxTextItem) string {
	var builder strings.Builder
	var previous *boxTextItem
	for index := range group {
		item := group[index]
		text := strings.ReplaceAll(item.text, "\n", " ")
		if previous != nil {
			if strings.TrimSpace(text) == strings.TrimSpace(previous.text) &&
				math.Abs(item.x-previous.x) < 0.5*math.Max(item.width, 0.1) {
				continue
			}
			gap := item.x - (previous.x + previous.width)
			current := builder.String()
			if gap > boxColumnGapRatio*item.height && !strings.HasSuffix(current, " ") && !strings.HasPrefix(text, " ") {
				builder.WriteString(" ")
			}
		}
		builder.WriteString(text)
		previous = &group[index]
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func boxCenter(item boxTextItem) float64 { return item.y + item.height/2 }

func groupCenter(group []boxTextItem) float64 {
	total := 0.0
	for _, item := range group {
		total += boxCenter(item)
	}
	return total / float64(len(group))
}

func medianBoxHeight(items []boxTextItem) float64 {
	heights := make([]float64, 0, len(items))
	for _, item := range items {
		if item.height > 0 {
			heights = append(heights, item.height)
		}
	}
	if len(heights) == 0 {
		return 0
	}
	sort.Float64s(heights)
	return heights[len(heights)/2]
}

// renderBoxTextLayer turns a decoded txt_all.json into Markdown, one section per page.
//
// It returns the rendered lines and the page count. Page keys are decimal strings,
// "0" through "N-1", and are ordered numerically: as strings "10" sorts before "2".
func renderBoxTextLayer(layer map[string]any) ([]string, int) {
	pagesByKey := asMap(layer["page"])
	keys := make([]string, 0, len(pagesByKey))
	for key := range pagesByKey {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(left, right int) bool {
		leftNumber, leftErr := strconv.Atoi(keys[left])
		rightNumber, rightErr := strconv.Atoi(keys[right])
		if leftErr == nil && rightErr == nil {
			return leftNumber < rightNumber
		}
		return keys[left] < keys[right]
	})

	parsed := make([][]boxTextItem, len(keys))
	everything := []boxTextItem{}
	for index, key := range keys {
		parsed[index] = parseBoxTextItems(asSlice(pagesByKey[key]))
		everything = append(everything, parsed[index]...)
	}
	// The median is a body glyph's height only because body glyphs are the
	// majority, which they are in any document with real text: a 46-page deck had
	// 29,631 inline glyphs against 480 tall boxes. A layer made almost entirely of
	// tall boxes would pull the median up and stop blocks reading as blocks.
	referenceHeight := medianBoxHeight(everything)
	tiles := watermarkTileSizes(parsed, referenceHeight)
	watermark := watermarkText(parsed, tiles)

	pageLines := make([][]boxTextLine, len(keys))
	for index := range keys {
		pageLines[index] = dropWatermarkFragments(
			layoutBoxPage(dropWatermarkTiles(parsed[index], tiles), referenceHeight), watermark)
	}
	repeated := repeatedBoxLines(pageLines)

	lines := []string{}
	for index := range keys {
		lines = append(lines, fmt.Sprintf("## Page %d", index+1), "")
		written := 0
		for _, line := range pageLines[index] {
			if repeated[line.text] {
				continue
			}
			lines = append(lines, escapeBoxMarkdownLine(line.text))
			written++
		}
		if written == 0 {
			lines = append(lines, "_(no text on this page)_")
		}
		lines = append(lines, "")
	}
	return lines, len(keys)
}

// repeatedBoxLines finds lines that recur verbatim across most pages.
func repeatedBoxLines(pages [][]boxTextLine) map[string]bool {
	repeated := map[string]bool{}
	if len(pages) < boxRepeatedLineMinPages {
		return repeated
	}
	seenOn := map[string]int{}
	for _, page := range pages {
		onThisPage := map[string]bool{}
		for _, line := range page {
			onThisPage[line.text] = true
		}
		for text := range onThisPage {
			seenOn[text]++
		}
	}
	threshold := int(math.Ceil(boxRepeatedLineShare * float64(len(pages))))
	for text, count := range seenOn {
		if count >= threshold {
			repeated[text] = true
		}
	}
	return repeated
}

// escapeBoxMarkdownLine stops page text from being read as Markdown structure.
//
// Two leading forms do real damage. A "#" turns body text into a heading that
// competes with the page headings, and a code fence swallows every page after it.
// List and quote markers are left alone: rendering them is harmless and they are
// usually what the text meant.
func escapeBoxMarkdownLine(text string) string {
	if strings.HasPrefix(text, "#") || strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~") {
		return `\` + text
	}
	return text
}

// boxTileSize is a box's width and height rounded to a tenth of a point, the
// precision at which the server reports a repeated tile identically.
type boxTileSize struct {
	width  int
	height int
}

func tileSizeOf(item boxTextItem) boxTileSize {
	return boxTileSize{width: int(math.Round(item.width * 10)), height: int(math.Round(item.height * 10))}
}

// watermarkTileSizes finds box sizes that are a tiled watermark rather than text.
//
// A diagonal watermark is stamped across every page in fixed-size tiles, and the
// extractor reports each tile as one tall box. Its signature is uniformity: the
// same width and height to a tenth of a point, many times over, on most pages.
// Real multi-line text blocks vary in size with their content, so a tall size that
// recurs identically on half the pages or more can only be the overlay. On a real
// 46-page deck all 392 such boxes measured exactly 118.1 by 125.3, and 383 of the
// lines found only in them were the watermark itself.
//
// Matching on geometry rather than on the watermark's text means nothing here
// knows what any tenant's watermark says.
func watermarkTileSizes(pages [][]boxTextItem, referenceHeight float64) map[boxTileSize]bool {
	tiles := map[boxTileSize]bool{}
	if referenceHeight <= 0 || len(pages) < boxRepeatedLineMinPages {
		return tiles
	}
	pagesWithSize := map[boxTileSize]int{}
	for _, page := range pages {
		onThisPage := map[boxTileSize]bool{}
		for _, item := range page {
			if item.height > boxBlockHeightRatio*referenceHeight {
				onThisPage[tileSizeOf(item)] = true
			}
		}
		for size := range onThisPage {
			pagesWithSize[size]++
		}
	}
	threshold := int(math.Ceil(boxRepeatedLineShare * float64(len(pages))))
	for size, count := range pagesWithSize {
		if count >= threshold {
			tiles[size] = true
		}
	}
	return tiles
}

func dropWatermarkTiles(items []boxTextItem, tiles map[boxTileSize]bool) []boxTextItem {
	if len(tiles) == 0 {
		return items
	}
	kept := make([]boxTextItem, 0, len(items))
	for _, item := range items {
		if !tiles[tileSizeOf(item)] {
			kept = append(kept, item)
		}
	}
	return kept
}

// watermarkText recovers what the tiled watermark says, from the tiles themselves.
//
// The tiles were found by geometry, and their text is mostly the watermark: the
// line that recurs most often across them is it. Recovering it this way keeps the
// code ignorant of any particular tenant's watermark.
func watermarkText(pages [][]boxTextItem, tiles map[boxTileSize]bool) string {
	if len(tiles) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, page := range pages {
		for _, item := range page {
			if !tiles[tileSizeOf(item)] {
				continue
			}
			for _, line := range strings.Split(item.text, "\n") {
				if trimmed := strings.TrimSpace(line); trimmed != "" {
					counts[trimmed]++
				}
			}
		}
	}
	best, bestCount := "", 0
	for line, count := range counts {
		if count > bestCount || (count == bestCount && line < best) {
			best, bestCount = line, count
		}
	}
	return best
}

// dropWatermarkFragments removes lines made only of a piece of the watermark.
//
// Where the watermark crosses empty space there is no body glyph for its glyphs to
// fuse into, so they stand alone and form lines of their own: "AI开发", "二期".
// A line that is a substring of the watermark is such a fragment. Two runes is the
// floor, so a single character that merely happens to occur in the watermark is
// kept.
func dropWatermarkFragments(lines []boxTextLine, watermark string) []boxTextLine {
	if watermark == "" {
		return lines
	}
	kept := lines[:0]
	for _, line := range lines {
		if len([]rune(line.text)) >= 2 && strings.Contains(watermark, line.text) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}
