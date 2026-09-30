package docslocal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// TreeConfig describes a wiki subtree listing.
type TreeConfig struct {
	Source      string
	CookiesPath string
	SpaceAPI    string
	// Recursive walks descendants instead of listing only direct children.
	Recursive bool
	// MaxDepth bounds a recursive walk. Depth 1 is the direct children.
	MaxDepth int
}

// TreeNode is one document in the listing.
type TreeNode struct {
	URL       string
	Title     string
	WikiToken string
	ObjToken  string
	ObjType   int
	Kind      string
	// Readable reports whether `ixf docs read` can read this node. Only docx-backed
	// nodes can; a wiki-hosted native sheet or an uploaded file cannot.
	Readable   bool
	HasChild   bool
	Depth      int
	ParentPath string
}

// wikiTreeNodeLimit bounds a recursive walk regardless of MaxDepth.
//
// Each node carrying children costs one request, because the tree endpoint expands
// only the requested node's children and no deeper. A wide subtree can therefore
// turn one command into hundreds of requests, so the walk stops and says it was
// truncated rather than continuing silently.
const wikiTreeNodeLimit = 500

// defaultWikiTreeMaxDepth applies when a recursive walk names no depth.
const defaultWikiTreeMaxDepth = 10

// ListWikiTree reports the documents under a wiki node.
//
// The default is the direct children only. Listing is usually a step before
// reading, and one level answers "what is in here" without paying for a walk whose
// request count cannot be predicted from the URL alone.
func ListWikiTree(config TreeConfig) ([]TreeNode, bool, error) {
	if remoteKindFromSource(config.Source) != "wiki" {
		return nil, false, fmt.Errorf("docs tree requires a /wiki/ URL; %s is not one", config.Source)
	}
	origin, rootToken, err := wikiOriginAndToken(config.Source)
	if err != nil {
		return nil, false, err
	}
	session, err := newRemoteReadSession(ReadOptions{
		CookiesPath: config.CookiesPath,
		SpaceAPI:    config.SpaceAPI,
	})
	if err != nil {
		return nil, false, err
	}
	spaceID, err := session.wikiSpaceID(config.Source, origin)
	if err != nil {
		return nil, false, err
	}

	maxDepth := config.MaxDepth
	if !config.Recursive {
		maxDepth = 1
	} else if maxDepth <= 0 {
		maxDepth = defaultWikiTreeMaxDepth
	}

	collected := []TreeNode{}
	truncated := false
	visited := map[string]bool{rootToken: true}

	type pending struct {
		token string
		depth int
		path  string
	}
	queue := []pending{{token: rootToken, depth: 0, path: ""}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= maxDepth {
			continue
		}
		children, err := session.wikiChildren(origin, spaceID, current.token)
		if err != nil {
			return nil, false, err
		}
		for _, child := range children {
			if visited[child.WikiToken] {
				continue
			}
			visited[child.WikiToken] = true
			child.Depth = current.depth + 1
			child.ParentPath = current.path
			if len(collected) >= wikiTreeNodeLimit {
				truncated = true
				return collected, truncated, nil
			}
			collected = append(collected, child)
			if child.HasChild {
				childPath := child.Title
				if current.path != "" {
					childPath = current.path + " / " + child.Title
				}
				queue = append(queue, pending{token: child.WikiToken, depth: child.Depth, path: childPath})
			}
		}
	}
	return collected, truncated, nil
}

// wikiChildren fetches one node's direct children.
//
// The endpoint returns the path from the space root down to the requested token
// plus that token's children, so a walk has to call it once per node rather than
// reading a whole subtree from a single response.
func (session *remoteReadSession) wikiChildren(origin string, spaceID string, token string) ([]TreeNode, error) {
	requestURL := fmt.Sprintf("%s/space/api/wiki/v2/tree/get_info/?space_id=%s&wiki_token=%s",
		origin, url.QueryEscape(spaceID), url.QueryEscape(token))
	payload, err := session.getJSON(requestURL, origin, origin+"/wiki/"+token)
	if err != nil {
		return nil, err
	}
	tree := asMap(asMap(payload["data"])["tree"])
	if len(tree) == 0 {
		return nil, fmt.Errorf("wiki tree response contained no tree for %s", token)
	}
	nodes := asMap(tree["nodes"])
	childTokens := wikiTokenList(asMap(tree["child_map"])[token])

	children := make([]TreeNode, 0, len(childTokens))
	for _, childToken := range childTokens {
		raw := asMap(nodes[childToken])
		if len(raw) == 0 {
			// The token is listed as a child but its detail is absent, which happens
			// for nodes the caller cannot see. Skipping keeps the listing to what is
			// actually reachable rather than emitting an entry with no URL.
			continue
		}
		children = append(children, treeNodeFromRaw(raw, origin, childToken))
	}
	return children, nil
}

// treeNodeFromRaw builds one node from a tree response entry.
//
// URL is absolute by construction. A caller feeds it straight back into
// `ixf docs read`, which needs a scheme and host, so a relative or absent value
// from the server is resolved against the origin rather than passed through. Live
// responses have only ever carried absolute URLs equal to origin + /wiki/ + token;
// this keeps the guarantee from resting on that.
func treeNodeFromRaw(raw map[string]any, origin string, token string) TreeNode {
	objType := intValue(raw["obj_type"])
	objToken := strings.TrimSpace(stringValue(raw["obj_token"]))
	kind := wikiObjKind(objToken, objType)
	nodeURL := strings.TrimSpace(stringValue(raw["url"]))
	if nodeURL == "" {
		nodeURL = origin + "/wiki/" + token
	} else if strings.HasPrefix(nodeURL, "/") {
		nodeURL = origin + nodeURL
	}
	return TreeNode{
		URL:       nodeURL,
		Title:     strings.TrimSpace(stringValue(raw["title"])),
		WikiToken: token,
		ObjToken:  objToken,
		ObjType:   objType,
		Kind:      kind,
		Readable:  wikiKindIsReadable(kind),
		HasChild:  readBool(raw["has_child"]),
	}
}

// wikiObjKind names a node's type from its object token prefix.
//
// The prefix is used rather than obj_type because it is self-describing and was
// confirmed against live data: dox with obj_type 22 is a docx, sht with 3 a native
// sheet, box with 12 an uploaded file. Mapping from the numeric code alone would
// mean maintaining a table of values observed on one tenant.
func wikiObjKind(objToken string, objType int) string {
	switch {
	case strings.HasPrefix(objToken, "dox"):
		return "docx"
	case strings.HasPrefix(objToken, "sht"):
		return "sheet"
	case strings.HasPrefix(objToken, "box"):
		return "file"
	case strings.HasPrefix(objToken, "bas"):
		return "bitable"
	case strings.HasPrefix(objToken, "bmn"):
		return "mindnote"
	default:
		return fmt.Sprintf("unknown(objType=%d)", objType)
	}
}

// wikiKindIsReadable reports whether `ixf docs read` can read this node.
//
// A docx-backed node reads through the docx path. A bitable-backed one reads as well,
// because the wiki read path fetches the page, recognizes bitable HTML and routes to the
// bitable reader before it looks for a docx token; both were confirmed against a live
// directory. A wiki-hosted native sheet and an uploaded file both fail with
// "client_vars failed", so reporting the kind without this would leave a caller to
// discover it by feeding the whole listing to docs read and watching part of it fail. A
// mindnote is treated as unreadable because a /wiki/ URL never reaches the mindnote
// branch and carries no docx token; that one is reasoned from the read path rather than
// live-confirmed.
func wikiKindIsReadable(kind string) bool {
	return kind == "docx" || kind == "bitable"
}

// wikiTokenList reads a child_map entry, which is a JSON array of token strings.
func wikiTokenList(value any) []string {
	tokens := []string{}
	for _, raw := range asSlice(value) {
		if token := stringValue(raw); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// wikiOriginAndToken splits a wiki URL into its origin and node token.
func wikiOriginAndToken(source string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed.Host == "" {
		return "", "", fmt.Errorf("wiki URL must be absolute HTTP(S)")
	}
	// The scheme is checked rather than merely required to be non-empty: the origin
	// built here is used for HTTP requests, so letting another scheme through means
	// failing later with a message about something else.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("wiki URL must be absolute HTTP(S); got scheme %q", parsed.Scheme)
	}
	token := tokenAfter(parsed.Path, "/wiki/")
	if token == "" {
		return "", "", fmt.Errorf("wiki URL is missing its node token")
	}
	return originForURL(parsed), token, nil
}

// wikiSpaceID reads the space identifier the tree endpoint requires.
//
// It is not in the URL: the page embeds it in a current_space_wiki object, and the
// first occurrence of that assignment is an empty one inside a guard, so the
// populated assignment is the one to read.
func (session *remoteReadSession) wikiSpaceID(source string, origin string) (string, error) {
	html, err := session.fetchHTML(source, origin)
	if err != nil {
		return "", err
	}
	const anchor = "window.current_space_wiki = Object({"
	start := strings.Index(html, anchor)
	if start < 0 {
		return "", fmt.Errorf("could not locate the wiki space identifier on the page")
	}
	start += len(anchor) - 1
	depth := 0
	end := -1
	for index := start; index < len(html); index++ {
		switch html[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = index + 1
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return "", fmt.Errorf("wiki space object on the page was not closed")
	}
	info := map[string]any{}
	if err := json.Unmarshal([]byte(html[start:end]), &info); err != nil {
		return "", fmt.Errorf("wiki space object did not parse: %w", err)
	}
	spaceID := stringValue(info["space_id"])
	if spaceID == "" {
		return "", fmt.Errorf("wiki page did not report a space identifier")
	}
	return spaceID, nil
}
