package docspublish

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/serialq7ic4/ixf-toolbox/internal/docxgraph"
)

func Structure(rawURL string, cookiesPath string, spaceAPI string) (map[string]any, error) {
	target, err := parsePatchTarget(rawURL)
	if err != nil {
		return nil, err
	}
	session, err := newPublishSession(Config{
		CookiesPath: cookiesPath,
		SpaceAPI:    spaceAPI,
	}, target.BaseURL)
	if err != nil {
		return nil, err
	}
	if target.Token == "" && target.Kind == "wiki" {
		token, err := session.resolveWikiDocxToken(target.Referer)
		if err != nil {
			return nil, err
		}
		target.Token = token
	}
	if target.Token == "" {
		return nil, fmt.Errorf("docs structure requires a docx URL or wiki-backed docx URL")
	}
	state, err := session.clientVars(target.Token, target.Referer)
	if err != nil {
		return nil, err
	}
	structure, err := safeStructureSummary(state, target.Token)
	if err != nil {
		return nil, err
	}
	title := titleFromState(state, target.Token)
	if title == "" {
		title = "Untitled"
	}
	return map[string]any{
		"ok":          true,
		"operation":   "docs_structure",
		"targetKind":  target.Kind,
		"targetToken": safeTokenSummary(target.Token),
		"title":       title,
		"structure":   structure,
	}, nil
}

func safeStructureSummary(state map[string]any, token string) (map[string]any, error) {
	graph, err := docxgraph.Build(state, token)
	if err != nil {
		return nil, err
	}
	return graph.SafeSummary(), nil
}

func titleFromState(state map[string]any, token string) string {
	rootData := dataForBlock(asMap(state["block_map"])[token])
	return textFromBlockData(rootData)
}

func safeTokenSummary(token string) map[string]any {
	if token == "" {
		return map[string]any{"id": "", "length": 0}
	}
	sum := sha256.Sum256([]byte(token))
	return map[string]any{
		"id":     "token_" + hex.EncodeToString(sum[:])[:12],
		"length": len(token),
	}
}
