package docxgraph

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type InsertPosition string

const (
	PositionSectionEnd   InsertPosition = "section-end"
	PositionAfterHeading InsertPosition = "after-heading"
)

var whitespacePattern = regexp.MustCompile(`\s+`)
var headingLevelPattern = regexp.MustCompile(`^heading(\d+)$`)

func NormalizeHeading(text string) string {
	text = strings.ReplaceAll(text, "\u3000", " ")
	text = whitespacePattern.ReplaceAllString(strings.TrimSpace(text), " ")
	return text
}

func (g Graph) FindHeadingByText(text string) (HeadingRef, error) {
	target := NormalizeHeading(text)
	matches := []HeadingRef{}
	for index, blockID := range g.RootChildren {
		block := g.Blocks[blockID]
		level := headingLevel(block.Kind)
		if level == 0 {
			continue
		}
		if NormalizeHeading(block.Text) == target {
			matches = append(matches, HeadingRef{
				ID:    block.ID,
				Text:  block.Text,
				Level: level,
				Index: index,
			})
		}
	}
	if len(matches) == 0 {
		return HeadingRef{}, fmt.Errorf("heading not found: %s", text)
	}
	if len(matches) > 1 {
		return HeadingRef{}, fmt.Errorf("ambiguous heading: %s", text)
	}
	return matches[0], nil
}

func (g Graph) SectionRange(anchor HeadingRef) SectionRange {
	end := len(g.RootChildren)
	for index := anchor.Index + 1; index < len(g.RootChildren); index++ {
		level := headingLevel(g.Blocks[g.RootChildren[index]].Kind)
		if level > 0 && level <= anchor.Level {
			end = index
			break
		}
	}
	return SectionRange{
		Start: anchor.Index,
		End:   end,
		IDs:   append([]string(nil), g.RootChildren[anchor.Index:end]...),
	}
}

func (g Graph) InsertIndex(anchor HeadingRef, position InsertPosition) (int, error) {
	switch position {
	case "", PositionSectionEnd:
		return g.SectionRange(anchor).End, nil
	case PositionAfterHeading:
		return anchor.Index + 1, nil
	default:
		return 0, fmt.Errorf("unsupported insert position: %s", position)
	}
}

func headingLevel(kind string) int {
	match := headingLevelPattern.FindStringSubmatch(kind)
	if len(match) != 2 {
		return 0
	}
	level, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return level
}
