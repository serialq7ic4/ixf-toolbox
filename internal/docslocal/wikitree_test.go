package docslocal

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// The object-token prefixes below are the shapes the live wiki tree endpoint sends:
// dox with obj_type 22 is a docx, sht with 3 a native sheet, box with 12 an uploaded
// file. wikiObjKind reads the prefix rather than the numeric code, so a node whose
// obj_type disagrees with its prefix is still named from the prefix, and a node with
// no usable prefix reports the numeric code instead of guessing from it.
func TestWikiObjKindNamesNodesFromTheObjectTokenPrefix(t *testing.T) {
	tests := []struct {
		name     string
		objToken string
		objType  int
		want     string
	}{
		{name: "docx", objToken: "doxcnFixtureToken", objType: 22, want: "docx"},
		{name: "native sheet", objToken: "shtcnFixtureToken", objType: 3, want: "sheet"},
		{name: "uploaded file", objToken: "boxcnFixtureToken", objType: 12, want: "file"},
		{name: "bitable", objToken: "bascnFixtureToken", objType: 8, want: "bitable"},
		{name: "mindnote", objToken: "bmncnFixtureToken", objType: 11, want: "mindnote"},
		{name: "prefix wins over a disagreeing obj_type", objToken: "doxcnFixtureToken", objType: 3, want: "docx"},
		{name: "prefix alone is enough", objToken: "dox", objType: 22, want: "docx"},
		{name: "unknown prefix reports the numeric type", objToken: "wikcnFixtureToken", objType: 99, want: "unknown(objType=99)"},
		{name: "empty token", objToken: "", objType: 0, want: "unknown(objType=0)"},
		{name: "empty token still surfaces the numeric type", objToken: "", objType: 22, want: "unknown(objType=22)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := wikiObjKind(test.objToken, test.objType); got != test.want {
				t.Fatalf("wikiObjKind(%q, %d) = %q, want %q", test.objToken, test.objType, got, test.want)
			}
		})
	}
}

// Two kinds read and the rest do not, each verified against a live directory. A docx reads
// through the docx path, and a bitable reads because readRemote takes the same /wiki/ URL,
// recognizes bitable HTML with isBitableWikiHTML and routes to readBitableWiki before it
// looks for a docx token. A native sheet and an uploaded file both fail there with
// "client_vars failed". A mindnote is unreadable by the same reasoning the read path gives:
// a /wiki/ URL never reaches the mindnote branch, so it falls through to a docx token
// extraction that finds nothing.
func TestWikiKindIsReadableAcceptsDocxAndBitableBackedNodes(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want bool
	}{
		{name: "docx", kind: "docx", want: true},
		{name: "bitable", kind: "bitable", want: true},
		{name: "native sheet", kind: "sheet", want: false},
		{name: "uploaded file", kind: "file", want: false},
		{name: "mindnote", kind: "mindnote", want: false},
		{name: "unknown", kind: "unknown(objType=99)", want: false},
		{name: "unknown with no type", kind: "unknown(objType=0)", want: false},
		{name: "empty kind", kind: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := wikiKindIsReadable(test.kind); got != test.want {
				t.Fatalf("wikiKindIsReadable(%q) = %v, want %v", test.kind, got, test.want)
			}
		})
	}
}

// A wiki URL is copied out of a browser, so it arrives with a query string, a fragment,
// a trailing slash or surrounding whitespace. Each of those has to yield the same origin
// and node token, and anything that is not an absolute wiki URL has to report an error
// rather than hand back empty strings a caller would use as a token.
func TestWikiOriginAndTokenSplitsAbsoluteWikiURLs(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		wantOrigin string
		wantToken  string
	}{
		{
			name:       "plain wiki URL",
			source:     "https://tenant.example.test/wiki/wikcnFixtureToken",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "query string",
			source:     "https://tenant.example.test/wiki/wikcnFixtureToken?from=copy",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "fragment",
			source:     "https://tenant.example.test/wiki/wikcnFixtureToken#section-2",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "query string and fragment",
			source:     "https://tenant.example.test/wiki/wikcnFixtureToken?from=copy#section-2",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "trailing slash",
			source:     "https://tenant.example.test/wiki/wikcnFixtureToken/",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "deeper path after the token",
			source:     "https://tenant.example.test/wiki/wikcnFixtureToken/edit",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "surrounding whitespace",
			source:     "  https://tenant.example.test/wiki/wikcnFixtureToken  ",
			wantOrigin: "https://tenant.example.test",
			wantToken:  "wikcnFixtureToken",
		},
		{
			name:       "host with a port",
			source:     "http://127.0.0.1:8080/wiki/wikcnFixtureToken",
			wantOrigin: "http://127.0.0.1:8080",
			wantToken:  "wikcnFixtureToken",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			origin, token, err := wikiOriginAndToken(test.source)
			if err != nil {
				t.Fatalf("wikiOriginAndToken(%q) returned error: %v", test.source, err)
			}
			if origin != test.wantOrigin || token != test.wantToken {
				t.Fatalf("wikiOriginAndToken(%q) = %q, %q; want %q, %q",
					test.source, origin, token, test.wantOrigin, test.wantToken)
			}
		})
	}
}

func TestWikiOriginAndTokenRejectsSourcesThatAreNotAbsoluteWikiURLs(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "non-wiki path", source: "https://tenant.example.test/docx/doxcnFixtureToken"},
		{name: "host root", source: "https://tenant.example.test/"},
		{name: "wiki path with no token", source: "https://tenant.example.test/wiki/"},
		{name: "wiki path with no trailing segment", source: "https://tenant.example.test/wiki"},
		{name: "relative path", source: "/wiki/wikcnFixtureToken"},
		{name: "schemeless host", source: "tenant.example.test/wiki/wikcnFixtureToken"},
		{name: "protocol relative", source: "//tenant.example.test/wiki/wikcnFixtureToken"},
		{name: "scheme with no host", source: "https:///wiki/wikcnFixtureToken"},
		// The origin is used for HTTP requests, so a non-HTTP scheme is rejected here
		// rather than left to fail later with a message about something else.
		{name: "ftp scheme", source: "ftp://tenant.example.test/wiki/wikcnFixtureToken"},
		{name: "file scheme", source: "file:///wiki/wikcnFixtureToken"},
		{name: "empty", source: ""},
		{name: "whitespace only", source: "   "},
		{name: "garbage", source: "not a url at all"},
		{name: "unparsable", source: "://nope"},
		{name: "invalid percent escape", source: "https://tenant.example.test/wiki/%zz"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			origin, token, err := wikiOriginAndToken(test.source)
			if err == nil {
				t.Fatalf("wikiOriginAndToken(%q) = %q, %q, nil; want an error", test.source, origin, token)
			}
			if origin != "" || token != "" {
				t.Fatalf("wikiOriginAndToken(%q) returned %q, %q alongside error %v; want empty values",
					test.source, origin, token, err)
			}
		})
	}
}

// A child_map entry arrives already decoded from the tree response, so it is a []any of
// token strings. Blank entries are dropped because a blank token cannot be looked up in
// the node map, and anything that is not an array degrades to an empty list rather than
// panicking or returning nil, so the caller iterates zero children instead of crashing on
// a response shape it did not expect.
func TestWikiTokenListReadsDecodedChildMapArrays(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{
			name:  "token array",
			value: []any{"wikcnFirst", "wikcnSecond", "wikcnThird"},
			want:  []string{"wikcnFirst", "wikcnSecond", "wikcnThird"},
		},
		{
			name:  "single token",
			value: []any{"wikcnOnly"},
			want:  []string{"wikcnOnly"},
		},
		{
			name:  "empty array",
			value: []any{},
			want:  []string{},
		},
		{
			name:  "blank entries are dropped",
			value: []any{"wikcnFirst", "", "wikcnSecond"},
			want:  []string{"wikcnFirst", "wikcnSecond"},
		},
		{
			name:  "nil entries are dropped",
			value: []any{"wikcnFirst", nil, "wikcnSecond"},
			want:  []string{"wikcnFirst", "wikcnSecond"},
		},
		{
			name:  "nil value",
			value: nil,
			want:  []string{},
		},
		{
			name:  "number instead of an array",
			value: float64(12),
			want:  []string{},
		},
		{
			name:  "object instead of an array",
			value: map[string]any{"wikcnFirst": true},
			want:  []string{},
		},
		{
			name:  "bare string instead of an array",
			value: "wikcnFirst",
			want:  []string{},
		},
		{
			name:  "serialized array instead of a decoded one",
			value: `["wikcnFirst","wikcnSecond"]`,
			want:  []string{},
		},
		{
			name:  "malformed serialized array",
			value: `["wikcnFirst",`,
			want:  []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := wikiTokenList(test.value)
			if got == nil {
				t.Fatalf("wikiTokenList(%#v) = nil, want an empty slice so the caller can range over it", test.value)
			}
			if len(got) != len(test.want) {
				t.Fatalf("wikiTokenList(%#v) = %#v, want %#v", test.value, got, test.want)
			}
			for index, want := range test.want {
				if got[index] != want {
					t.Fatalf("wikiTokenList(%#v)[%d] = %q, want %q; all %#v", test.value, index, got[index], want, got)
				}
			}
		})
	}
}

// The node objects below mirror what the tree endpoint sends: obj_type arrives as a JSON
// number, has_child as a bool, and url as the wiki URL of the node. A node whose url is
// absent still needs a usable URL, so it is composed from the origin and the wiki token,
// and a node missing every key has to degrade to empty fields instead of panicking.
func TestTreeNodeFromRawBuildsNodesFromTheTreeResponseShape(t *testing.T) {
	const origin = "https://tenant.example.test"
	tests := []struct {
		name  string
		raw   map[string]any
		token string
		want  TreeNode
	}{
		{
			name: "docx node",
			raw: map[string]any{
				"title":     "Release Plan",
				"obj_token": "doxcnFixtureToken",
				"obj_type":  float64(22),
				"url":       "https://tenant.example.test/wiki/wikcnFixtureToken",
				"has_child": true,
			},
			token: "wikcnFixtureToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFixtureToken",
				Title:     "Release Plan",
				WikiToken: "wikcnFixtureToken",
				ObjToken:  "doxcnFixtureToken",
				ObjType:   22,
				Kind:      "docx",
				Readable:  true,
				HasChild:  true,
			},
		},
		{
			name: "native sheet node",
			raw: map[string]any{
				"title":     "Capacity",
				"obj_token": "shtcnFixtureToken",
				"obj_type":  float64(3),
				"has_child": false,
			},
			token: "wikcnSheetToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnSheetToken",
				Title:     "Capacity",
				WikiToken: "wikcnSheetToken",
				ObjToken:  "shtcnFixtureToken",
				ObjType:   3,
				Kind:      "sheet",
				Readable:  false,
				HasChild:  false,
			},
		},
		{
			name: "uploaded file node",
			raw: map[string]any{
				"title":     "架构图.pdf",
				"obj_token": "boxcnFixtureToken",
				"obj_type":  float64(12),
			},
			token: "wikcnFileToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFileToken",
				Title:     "架构图.pdf",
				WikiToken: "wikcnFileToken",
				ObjToken:  "boxcnFixtureToken",
				ObjType:   12,
				Kind:      "file",
				Readable:  false,
				HasChild:  false,
			},
		},
		{
			name: "bitable node",
			raw: map[string]any{
				"title":     "Rollout Tracker",
				"obj_token": "bascnFixtureToken",
				"obj_type":  float64(8),
				"has_child": false,
			},
			token: "wikcnBitableToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnBitableToken",
				Title:     "Rollout Tracker",
				WikiToken: "wikcnBitableToken",
				ObjToken:  "bascnFixtureToken",
				ObjType:   8,
				Kind:      "bitable",
				Readable:  true,
				HasChild:  false,
			},
		},
		{
			name: "padded title and token are trimmed",
			raw: map[string]any{
				"title":     "  Release Plan  ",
				"obj_token": "  doxcnFixtureToken  ",
				"obj_type":  float64(22),
			},
			token: "wikcnFixtureToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFixtureToken",
				Title:     "Release Plan",
				WikiToken: "wikcnFixtureToken",
				ObjToken:  "doxcnFixtureToken",
				ObjType:   22,
				Kind:      "docx",
				Readable:  true,
			},
		},
		{
			name: "blank url falls back to the wiki URL",
			raw: map[string]any{
				"title":     "Release Plan",
				"obj_token": "doxcnFixtureToken",
				"obj_type":  float64(22),
				"url":       "   ",
			},
			token: "wikcnFixtureToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFixtureToken",
				Title:     "Release Plan",
				WikiToken: "wikcnFixtureToken",
				ObjToken:  "doxcnFixtureToken",
				ObjType:   22,
				Kind:      "docx",
				Readable:  true,
			},
		},
		{
			name: "string obj_type",
			raw: map[string]any{
				"title":     "Release Plan",
				"obj_token": "doxcnFixtureToken",
				"obj_type":  "22",
			},
			token: "wikcnFixtureToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFixtureToken",
				Title:     "Release Plan",
				WikiToken: "wikcnFixtureToken",
				ObjToken:  "doxcnFixtureToken",
				ObjType:   22,
				Kind:      "docx",
				Readable:  true,
			},
		},
		{
			name:  "empty node degrades instead of panicking",
			raw:   map[string]any{},
			token: "wikcnFixtureToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFixtureToken",
				WikiToken: "wikcnFixtureToken",
				Kind:      "unknown(objType=0)",
			},
		},
		{
			name:  "nil values degrade instead of panicking",
			raw:   map[string]any{"title": nil, "obj_token": nil, "obj_type": nil, "url": nil, "has_child": nil},
			token: "wikcnFixtureToken",
			want: TreeNode{
				URL:       "https://tenant.example.test/wiki/wikcnFixtureToken",
				WikiToken: "wikcnFixtureToken",
				Kind:      "unknown(objType=0)",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := treeNodeFromRaw(test.raw, origin, test.token)
			if got != test.want {
				t.Fatalf("treeNodeFromRaw = %#v, want %#v", got, test.want)
			}
			if got.Readable != (got.Kind == "docx" || got.Kind == "bitable") {
				t.Fatalf("Readable = %v disagrees with Kind = %q", got.Readable, got.Kind)
			}
			if got.Depth != 0 || got.ParentPath != "" {
				t.Fatalf("Depth = %d and ParentPath = %q, want the walk to own placement", got.Depth, got.ParentPath)
			}
		})
	}
}

// has_child decides whether the walk queues another request for a node, so every truthy
// and falsy shape the response can carry has to land on the right side.
func TestTreeNodeFromRawReadsHasChildAcrossResponseShapes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "bool true", value: true, want: true},
		{name: "bool false", value: false, want: false},
		{name: "number one", value: float64(1), want: true},
		{name: "number zero", value: float64(0), want: false},
		{name: "string true", value: "true", want: true},
		{name: "string false", value: "false", want: false},
		{name: "absent", value: nil, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := map[string]any{"obj_token": "doxcnFixtureToken", "obj_type": float64(22)}
			if test.value != nil {
				raw["has_child"] = test.value
			}
			if got := treeNodeFromRaw(raw, "https://tenant.example.test", "wikcnFixtureToken").HasChild; got != test.want {
				t.Fatalf("HasChild for has_child=%#v = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

// TreeNode.URL is fed back into `ixf docs read`, which requires an absolute URL:
// wikiOriginAndToken rejects an empty scheme or host, and originForURL yields "://" for a
// relative input. The absent-url branch already composes an absolute URL from the origin, so
// a relative value is resolved the same way rather than passed through.
//
// Defensive rather than observed: a live listing of 62 nodes carried only absolute URLs, each
// equal to origin + "/wiki/" + token. This keeps the guarantee from resting on that.
func TestTreeNodeFromRawResolvesARelativeNodeURLAgainstTheOrigin(t *testing.T) {
	raw := map[string]any{
		"title":     "Release Plan",
		"obj_token": "doxcnFixtureToken",
		"obj_type":  float64(22),
		"url":       "/wiki/wikcnFixtureToken",
	}
	got := treeNodeFromRaw(raw, "https://tenant.example.test", "wikcnFixtureToken")
	if got.URL != "https://tenant.example.test/wiki/wikcnFixtureToken" {
		t.Fatalf("URL = %q, want an absolute URL `ixf docs read` accepts", got.URL)
	}
}

// This replaces an earlier scratch probe that listed a real wiki over the network. The
// shapes below are the ones that probe confirmed: the space identifier is embedded in the
// page rather than the URL and its first assignment is an empty guard, the tree response
// carries the node detail in "nodes" and the parent-to-children edges in "child_map", and a
// token listed as a child can have no entry in "nodes" when the caller cannot see it.
func TestListWikiTreeListsDirectChildrenFromOneRequest(t *testing.T) {
	requested := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.String())
		switch r.URL.Path {
		case "/wiki/wikcnRootToken":
			_, _ = w.Write([]byte(wikiSpacePageFixture("7100")))
		case "/space/api/wiki/v2/tree/get_info/":
			if got := r.URL.Query().Get("space_id"); got != "7100" {
				t.Fatalf("space_id = %q, want 7100", got)
			}
			if got := r.URL.Query().Get("wiki_token"); got != "wikcnRootToken" {
				t.Fatalf("wiki_token = %q, want wikcnRootToken", got)
			}
			if got := r.Header.Get("X-CSRFToken"); got != "csrf-fixture" {
				t.Fatalf("X-CSRFToken = %q, want csrf-fixture", got)
			}
			writeJSONResponse(t, w, wikiTreeResponseFixture("wikcnRootToken", map[string]any{
				"wikcnDocToken": map[string]any{
					"title": "Release Plan", "obj_token": "doxcnFixtureToken",
					"obj_type": 22, "has_child": true,
				},
				"wikcnSheetToken": map[string]any{
					"title": "Capacity", "obj_token": "shtcnFixtureToken",
					"obj_type": 3, "has_child": false,
				},
				"wikcnFileToken": map[string]any{
					"title": "架构图.pdf", "obj_token": "boxcnFixtureToken",
					"obj_type": 12, "has_child": false,
				},
				"wikcnBitableToken": map[string]any{
					"title": "数据表", "obj_token": "bascnFixtureToken",
					"obj_type": 8, "has_child": false,
				},
			}, []string{"wikcnDocToken", "wikcnSheetToken", "wikcnFileToken", "wikcnBitableToken", "wikcnHiddenToken"}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	nodes, truncated, err := ListWikiTree(TreeConfig{
		Source:      server.URL + "/wiki/wikcnRootToken",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
	})
	if err != nil {
		t.Fatalf("ListWikiTree returned error: %v", err)
	}

	if truncated {
		t.Fatal("truncated = true, want false for a three node listing")
	}
	if len(nodes) != 4 {
		t.Fatalf("nodes = %#v, want four visible children", nodes)
	}
	want := []TreeNode{
		{
			URL: server.URL + "/wiki/wikcnDocToken", Title: "Release Plan",
			WikiToken: "wikcnDocToken", ObjToken: "doxcnFixtureToken", ObjType: 22,
			Kind: "docx", Readable: true, HasChild: true, Depth: 1,
		},
		{
			URL: server.URL + "/wiki/wikcnSheetToken", Title: "Capacity",
			WikiToken: "wikcnSheetToken", ObjToken: "shtcnFixtureToken", ObjType: 3,
			Kind: "sheet", Readable: false, HasChild: false, Depth: 1,
		},
		{
			URL: server.URL + "/wiki/wikcnFileToken", Title: "架构图.pdf",
			WikiToken: "wikcnFileToken", ObjToken: "boxcnFixtureToken", ObjType: 12,
			Kind: "file", Readable: false, HasChild: false, Depth: 1,
		},
		{
			URL: server.URL + "/wiki/wikcnBitableToken", Title: "数据表",
			WikiToken: "wikcnBitableToken", ObjToken: "bascnFixtureToken", ObjType: 8,
			Kind: "bitable", Readable: true, HasChild: false, Depth: 1,
		},
	}
	for index, wantNode := range want {
		if nodes[index] != wantNode {
			t.Fatalf("nodes[%d] = %#v, want %#v", index, nodes[index], wantNode)
		}
	}
	if len(requested) != 2 {
		t.Fatalf("requested = %#v, want the page and one tree request", requested)
	}
}

func TestListWikiTreeWalksDescendantsWithDepthAndParentPath(t *testing.T) {
	treeRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/wiki/wikcnRootToken":
			_, _ = w.Write([]byte(wikiSpacePageFixture("7100")))
		case r.URL.Path == "/space/api/wiki/v2/tree/get_info/" && r.URL.Query().Get("wiki_token") == "wikcnRootToken":
			treeRequests++
			writeJSONResponse(t, w, wikiTreeResponseFixture("wikcnRootToken", map[string]any{
				"wikcnDocToken": map[string]any{
					"title": "Release Plan", "obj_token": "doxcnFixtureToken",
					"obj_type": 22, "has_child": true,
				},
			}, []string{"wikcnDocToken"}))
		case r.URL.Path == "/space/api/wiki/v2/tree/get_info/" && r.URL.Query().Get("wiki_token") == "wikcnDocToken":
			treeRequests++
			writeJSONResponse(t, w, wikiTreeResponseFixture("wikcnDocToken", map[string]any{
				"wikcnChildToken": map[string]any{
					"title": "Rollback", "obj_token": "doxcnChildToken",
					"obj_type": 22, "has_child": false,
				},
			}, []string{"wikcnChildToken"}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	nodes, truncated, err := ListWikiTree(TreeConfig{
		Source:      server.URL + "/wiki/wikcnRootToken",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
		Recursive:   true,
	})
	if err != nil {
		t.Fatalf("ListWikiTree returned error: %v", err)
	}

	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes = %#v, want the child and the grandchild", nodes)
	}
	if nodes[0].Depth != 1 || nodes[0].ParentPath != "" {
		t.Fatalf("nodes[0] depth = %d, parentPath = %q; want 1 and empty", nodes[0].Depth, nodes[0].ParentPath)
	}
	if nodes[1].WikiToken != "wikcnChildToken" || nodes[1].Depth != 2 || nodes[1].ParentPath != "Release Plan" {
		t.Fatalf("nodes[1] = %#v, want the grandchild at depth 2 under \"Release Plan\"", nodes[1])
	}
	if treeRequests != 2 {
		t.Fatalf("tree requests = %d, want one per node carrying children", treeRequests)
	}
}

func TestListWikiTreeStopsAtTheRequestedDepth(t *testing.T) {
	treeRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/wiki/wikcnRootToken" {
			_, _ = w.Write([]byte(wikiSpacePageFixture("7100")))
			return
		}
		if r.URL.Path != "/space/api/wiki/v2/tree/get_info/" {
			http.NotFound(w, r)
			return
		}
		treeRequests++
		token := r.URL.Query().Get("wiki_token")
		writeJSONResponse(t, w, wikiTreeResponseFixture(token, map[string]any{
			token + "Child": map[string]any{
				"title": "Level " + token, "obj_token": "doxcn" + token,
				"obj_type": 22, "has_child": true,
			},
		}, []string{token + "Child"}))
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)

	nodes, _, err := ListWikiTree(TreeConfig{
		Source:      server.URL + "/wiki/wikcnRootToken",
		CookiesPath: cookiesPath,
		SpaceAPI:    server.URL,
		Recursive:   true,
		MaxDepth:    2,
	})
	if err != nil {
		t.Fatalf("ListWikiTree returned error: %v", err)
	}

	if len(nodes) != 2 {
		t.Fatalf("nodes = %#v, want two levels for MaxDepth 2", nodes)
	}
	if nodes[0].Depth != 1 || nodes[1].Depth != 2 {
		t.Fatalf("depths = %d and %d, want 1 and 2", nodes[0].Depth, nodes[1].Depth)
	}
	if treeRequests != 2 {
		t.Fatalf("tree requests = %d, want the walk to stop once depth 2 is listed", treeRequests)
	}
}

func TestListWikiTreeRejectsSourcesThatAreNotWikiURLs(t *testing.T) {
	for _, source := range []string{
		"https://tenant.example.test/docx/doxcnFixtureToken",
		"https://tenant.example.test/sheets/shtcnFixtureToken",
		"/local/path.md",
	} {
		t.Run(source, func(t *testing.T) {
			nodes, truncated, err := ListWikiTree(TreeConfig{Source: source})
			if err == nil {
				t.Fatalf("ListWikiTree(%q) = %#v, %v, nil; want an error", source, nodes, truncated)
			}
			if nodes != nil {
				t.Fatalf("ListWikiTree(%q) returned %#v alongside error %v; want no nodes", source, nodes, err)
			}
		})
	}
}

// wikiSpacePageFixture mirrors the wiki page the reader scrapes for the space identifier.
// The empty guard assignment comes first, exactly as the live page has it, so a reader that
// took the first assignment would find no identifier.
func wikiSpacePageFixture(spaceID string) string {
	return `<html><script>
		window.current_space_wiki = Object();
		window.current_space_wiki = Object({"space_id":"` + spaceID + `","name":"Fixture Space"});
	</script></html>`
}

// wikiTreeResponseFixture mirrors the tree endpoint: node detail in "nodes", parent to
// children edges in "child_map". A token in childTokens with no entry in nodes stands for a
// child the caller cannot see.
func wikiTreeResponseFixture(token string, nodes map[string]any, childTokens []string) map[string]any {
	children := make([]any, 0, len(childTokens))
	for _, childToken := range childTokens {
		children = append(children, childToken)
	}
	return map[string]any{
		"code": 0,
		"data": map[string]any{
			"tree": map[string]any{
				"nodes":     nodes,
				"child_map": map[string]any{token: children},
			},
		},
	}
}
