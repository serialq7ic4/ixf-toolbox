package docxgraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

type Block struct {
	ID       string
	Kind     string
	ParentID string
	Text     string
	Children []string
	Version  int
	Raw      map[string]any
}

type Graph struct {
	RootID       string
	RootVersion  int
	Blocks       map[string]Block
	RootChildren []string
}

type HeadingRef struct {
	ID    string
	Text  string
	Level int
	Index int
}

type SectionRange struct {
	Start int
	End   int
	IDs   []string
}

func Build(clientVars map[string]any, objToken string) (Graph, error) {
	rawMap := asMap(clientVars["block_map"])
	if len(rawMap) == 0 {
		return Graph{}, fmt.Errorf("client_vars block_map is empty")
	}
	blocks := map[string]Block{}
	for id, entryValue := range rawMap {
		entry := asMap(entryValue)
		data := asMap(entry["data"])
		if len(data) == 0 {
			data = entry
		}
		kind := stringValue(firstNonEmpty(data["type"], entry["type"]))
		if kind == "" {
			kind = "unknown"
		}
		blocks[id] = Block{
			ID:       id,
			Kind:     kind,
			ParentID: stringValue(firstNonEmpty(data["parent_id"], entry["parent_id"])),
			Text:     extractText(data["text"]),
			Children: readChildren(firstNonEmpty(data["children"], entry["children"])),
			Version:  intValue(entry["version"]),
			Raw:      data,
		}
	}
	rootID := findRootID(blocks, objToken)
	if rootID == "" {
		return Graph{}, fmt.Errorf("could not locate root block")
	}
	root := blocks[rootID]
	rootChildren := make([]string, 0, len(root.Children))
	for _, childID := range root.Children {
		if _, ok := blocks[childID]; ok {
			rootChildren = append(rootChildren, childID)
		}
	}
	return Graph{
		RootID:       rootID,
		RootVersion:  root.Version,
		Blocks:       blocks,
		RootChildren: rootChildren,
	}, nil
}

func findRootID(blocks map[string]Block, objToken string) string {
	if _, ok := blocks[objToken]; ok {
		return objToken
	}
	for id, block := range blocks {
		if block.Kind == "page" {
			return id
		}
	}
	for id, block := range blocks {
		if block.ParentID == "" {
			return id
		}
	}
	return ""
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				return value
			}
		case nil:
		default:
			return value
		}
	}
	return nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	default:
		return 0
	}
}

func extractText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if initial := asMap(typed["initialAttributedTexts"]); len(initial) > 0 {
			return textPieces(asMap(initial["text"]))
		}
		return textPieces(asMap(typed["text"]))
	default:
		return ""
	}
}

func textPieces(pieces map[string]any) string {
	if len(pieces) == 0 {
		return ""
	}
	keys := make([]int, 0, len(pieces))
	keyByInt := map[int]string{}
	for key := range pieces {
		parsed, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		keys = append(keys, parsed)
		keyByInt[parsed] = key
	}
	sort.Ints(keys)
	text := ""
	for _, key := range keys {
		text += stringValue(pieces[keyByInt[key]])
	}
	return text
}

func readChildren(value any) []string {
	switch typed := value.(type) {
	case []any:
		children := make([]string, 0, len(typed))
		for _, item := range typed {
			if child := stringValue(item); child != "" {
				children = append(children, child)
			}
		}
		return children
	case []string:
		return append([]string(nil), typed...)
	default:
		return []string{}
	}
}
