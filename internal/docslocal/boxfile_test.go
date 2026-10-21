package docslocal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// boxFileServer fakes the three requests a file read makes and records them.
//
// actions is what the permission service reports. A nil actions makes the
// permission call fail outright. textLayer controls whether file info advertises a
// text layer.
type boxFileServer struct {
	actions   map[string]any
	textLayer bool
	paths     []string
}

func (fake *boxFileServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fake.paths = append(fake.paths, r.URL.Path)
		switch {
		case r.URL.Path == "/space/api/suite/permission/document/actions/state/":
			if fake.actions == nil {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{"actions": fake.actions}})
		case r.URL.Path == "/space/api/box/file/info/":
			extra := map[string]any{"has_txt_all": fake.textLayer, "page_nums": 2}
			encoded, _ := json.Marshal(extra)
			writeJSONResponse(t, w, map[string]any{"code": 0, "data": map[string]any{
				"name":         "Release Plan.pdf",
				"type":         "pdf",
				"data_version": "7001",
				"preview_meta": map[string]any{"data": map[string]any{
					"22": map[string]any{"status": 0, "extra": string(encoded)},
				}},
			}})
		case strings.HasPrefix(r.URL.Path, "/space/api/box/stream/download/preview_sub/"):
			if got := r.URL.Query().Get("version"); got != "7001" {
				t.Fatalf("text layer version = %q, want the file's data_version", got)
			}
			writeJSONResponse(t, w, map[string]any{
				"link": []any{},
				"page": map[string]any{
					"0": []any{glyph("Goals", 10, 200, 20, 4)},
					"1": []any{glyph("Rollout", 10, 200, 28, 4)},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}
}

func (fake *boxFileServer) fetchedTextLayer() bool {
	for _, path := range fake.paths {
		if strings.Contains(path, "/preview_sub/") {
			return true
		}
	}
	return false
}

func readFakeBoxFile(t *testing.T, fake *boxFileServer, options ReadOptions) (Result, error) {
	t.Helper()
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCookieFixture(t, cookiesPath)
	options.CookiesPath = cookiesPath
	options.SpaceAPI = server.URL
	results, err := ReadSourcesWithOptions([]string{server.URL + "/file/boxcnFixtureToken"}, options)
	if err != nil {
		return Result{}, err
	}
	return results[0], nil
}

var copyAllowed = map[string]any{"view": 1, "preview": 1, "copy": 1}
var copyRestricted = map[string]any{"view": 1, "preview": 1, "copy": 102}

func TestRemoteKindFromPathRecognizesFileLinks(t *testing.T) {
	if got := remoteKindFromPath("/file/boxcnFixtureToken"); got != "file" {
		t.Fatalf("kind = %q, want file", got)
	}
	if got := remoteKindFromPath("/file/"); got != "remote" {
		t.Fatalf("a file path with no token is kind %q, want remote", got)
	}
}

func TestReadBoxFileReadsATextLayerWhenCopyingIsAllowed(t *testing.T) {
	fake := &boxFileServer{actions: copyAllowed, textLayer: true}
	result, err := readFakeBoxFile(t, fake, ReadOptions{})
	if err != nil {
		t.Fatalf("read returned error: %v", err)
	}
	if result.Kind != "file" || result.Title != "Release Plan.pdf" {
		t.Fatalf("kind/title = %q/%q, want file/Release Plan.pdf", result.Kind, result.Title)
	}
	for _, want := range []string{"## Page 1", "Goals", "## Page 2", "Rollout", "pages=2"} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("content missing %q:\n%s", want, result.Content)
		}
	}
	if len(result.Warnings) != 0 || strings.Contains(result.Content, "ignore-copy-restriction") {
		t.Fatalf("an unrestricted file carries a copy notice: %v\n%s", result.Warnings, result.Content)
	}
}

// The default must refuse a copy-restricted file, and refuse before the text layer
// is fetched: a refusal that arrives after the content was already pulled has not
// respected anything.
func TestReadBoxFileRefusesACopyRestrictedFileByDefault(t *testing.T) {
	fake := &boxFileServer{actions: copyRestricted, textLayer: true}
	_, err := readFakeBoxFile(t, fake, ReadOptions{})
	if err == nil || !strings.Contains(err.Error(), "disabled copying") ||
		!strings.Contains(err.Error(), "--ignore-copy-restriction") {
		t.Fatalf("err = %v, want a refusal naming the restriction and the flag", err)
	}
	if fake.fetchedTextLayer() {
		t.Fatal("the text layer was fetched before refusing")
	}
}

// With the flag, the file reads, and the result says so in its content as well as
// its warnings, so the context travels with the text.
func TestReadBoxFileReadsACopyRestrictedFileWhenAsked(t *testing.T) {
	fake := &boxFileServer{actions: copyRestricted, textLayer: true}
	result, err := readFakeBoxFile(t, fake, ReadOptions{IgnoreCopyRestriction: true})
	if err != nil {
		t.Fatalf("read returned error: %v", err)
	}
	if !strings.Contains(result.Content, "Goals") {
		t.Fatalf("content missing the text:\n%s", result.Content)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "disabled copying") {
		t.Fatalf("warnings = %v, want one naming the copy restriction", result.Warnings)
	}
	if !strings.Contains(result.Content, "> Read with --ignore-copy-restriction") {
		t.Fatalf("content does not carry the notice:\n%s", result.Content)
	}
}

// Viewing is the floor. The flag overrides a copy restriction, never a missing
// right to see the file at all.
func TestReadBoxFileRefusesWithoutViewPermissionEvenWithTheFlag(t *testing.T) {
	for name, actions := range map[string]map[string]any{
		"no view":    {"view": 2, "preview": 1, "copy": 1},
		"no preview": {"view": 1, "preview": 2, "copy": 1},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &boxFileServer{actions: actions, textLayer: true}
			_, err := readFakeBoxFile(t, fake, ReadOptions{IgnoreCopyRestriction: true})
			if err == nil || !strings.Contains(err.Error(), "permission to view") {
				t.Fatalf("err = %v, want a view refusal", err)
			}
			if fake.fetchedTextLayer() {
				t.Fatal("the text layer was fetched without view permission")
			}
		})
	}
}

// A permission answer that cannot be read is not permission. Failing closed is the
// rule everywhere else in this tool, and a guess here would read restricted files.
func TestReadBoxFileFailsClosedWhenPermissionIsUnknown(t *testing.T) {
	fake := &boxFileServer{actions: nil, textLayer: true}
	_, err := readFakeBoxFile(t, fake, ReadOptions{IgnoreCopyRestriction: true})
	if err == nil || !strings.Contains(err.Error(), "could not confirm permission") {
		t.Fatalf("err = %v, want a fail-closed refusal", err)
	}
	if fake.fetchedTextLayer() {
		t.Fatal("the text layer was fetched with permission unknown")
	}
}

// A permission code other than the one known affirmative is not allowed. 102 is
// what the server reports for an owner-imposed copy restriction.
func TestReadBoxFileTreatsAnyNonAllowedCopyCodeAsRestricted(t *testing.T) {
	for _, code := range []int{0, 2, 102, 999} {
		fake := &boxFileServer{actions: map[string]any{"view": 1, "preview": 1, "copy": code}, textLayer: true}
		if _, err := readFakeBoxFile(t, fake, ReadOptions{}); err == nil {
			t.Fatalf("copy code %d was read as allowed", code)
		}
	}
}

// An image-only scan has no text layer. Saying so beats returning an empty page.
func TestReadBoxFileReportsAMissingTextLayer(t *testing.T) {
	fake := &boxFileServer{actions: copyAllowed, textLayer: false}
	_, err := readFakeBoxFile(t, fake, ReadOptions{})
	if err == nil || !strings.Contains(err.Error(), "no server-generated text layer") {
		t.Fatalf("err = %v, want a missing text layer error", err)
	}
}
