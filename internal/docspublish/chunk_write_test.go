package docspublish

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// splitPublishServer is a fake editor endpoint that records each write's
// change_map size and enforces the measured server limit.
type splitPublishServer struct {
	mu            sync.Mutex
	changeSizes   []int
	rootChildren  []any
	rootVersion   int
	failAtWrite   int
	writeAttempts int
}

func (s *splitPublishServer) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/space/api/explorer/v2/create/object/":
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"obj_token": "doxrzCreatedPage"}})
		case "/space/api/docx/pages/client_vars":
			s.mu.Lock()
			children := append([]any{}, s.rootChildren...)
			version := s.rootVersion
			s.mu.Unlock()
			writeTestJSON(t, w, map[string]any{
				"code": 0,
				"data": map[string]any{
					"block_map": map[string]any{
						"doxrzCreatedPage": map[string]any{
							"version": version,
							"data": map[string]any{
								"type":     "page",
								"author":   "author_fixture",
								"children": children,
							},
						},
					},
				},
			})
		case "/space/api/docx/blocks/user_change/":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			changeMap := asMap(payload["change_map"])

			s.mu.Lock()
			s.writeAttempts++
			attempt := s.writeAttempts
			s.changeSizes = append(s.changeSizes, len(changeMap))
			s.mu.Unlock()

			if s.failAtWrite > 0 && attempt == s.failAtWrite {
				writeTestJSON(t, w, map[string]any{"code": invalidParamCode, "msg": "invalid param"})
				return
			}
			// Enforce the real server behavior so an over-budget write fails here too.
			if len(changeMap) > maxChangeEntriesPerWrite {
				writeTestJSON(t, w, map[string]any{"code": invalidParamCode, "msg": "invalid param"})
				return
			}
			s.mu.Lock()
			for id := range changeMap {
				if id == "doxrzCreatedPage" {
					continue
				}
				s.rootChildren = append(s.rootChildren, id)
			}
			s.rootVersion++
			s.mu.Unlock()
			writeTestJSON(t, w, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	}
}

func writeLargeMarkdown(t *testing.T, paragraphs int) string {
	t.Helper()
	builder := strings.Builder{}
	builder.WriteString("# Split Publish Fixture\n\n")
	for index := 0; index < paragraphs; index++ {
		builder.WriteString(fmt.Sprintf("Paragraph %d.\n\n", index))
	}
	path := filepath.Join(t.TempDir(), "large.md")
	if err := os.WriteFile(path, []byte(builder.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFixtureCookies(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := os.WriteFile(path, []byte(`[
		{"name":"_csrf_token","value":"csrf-fixture"},
		{"name":"session","value":"session-fixture"}
	]`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A document past the per-write budget must publish across several writes, each
// within the measured server limit.
func TestPublishMarkdownSplitsOversizedDocumentAcrossWrites(t *testing.T) {
	paragraphs := blockEntryWriteBudget + 250
	markdownPath := writeLargeMarkdown(t, paragraphs)
	fake := &splitPublishServer{}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	result, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      server.URL,
		CookiesPath:  writeFixtureCookies(t),
		SpaceAPI:     server.URL,
		Apply:        true,
	})
	if err != nil {
		t.Fatalf("split publish failed: %v", err)
	}
	if result["writeCount"] != 2 {
		t.Fatalf("writeCount = %v, want 2", result["writeCount"])
	}
	if result["splitWrites"] != true {
		t.Fatal("expected splitWrites to be reported")
	}
	if len(fake.changeSizes) != 2 {
		t.Fatalf("expected 2 writes, got %d: %v", len(fake.changeSizes), fake.changeSizes)
	}
	total := 0
	for _, size := range fake.changeSizes {
		if size > maxChangeEntriesPerWrite {
			t.Fatalf("a write carried %d change entries, over the %d limit", size, maxChangeEntriesPerWrite)
		}
		// Each write carries the page root entry plus its blocks.
		total += size - 1
	}
	if total != paragraphs {
		t.Fatalf("writes covered %d blocks, want %d", total, paragraphs)
	}
}

// An ordinary document must still take exactly one write, unchanged from before.
func TestPublishMarkdownKeepsSingleWriteForOrdinaryDocument(t *testing.T) {
	markdownPath := writeLargeMarkdown(t, 12)
	fake := &splitPublishServer{}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	result, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      server.URL,
		CookiesPath:  writeFixtureCookies(t),
		SpaceAPI:     server.URL,
		Apply:        true,
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if len(fake.changeSizes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(fake.changeSizes))
	}
	if result["writeCount"] != 1 {
		t.Fatalf("writeCount = %v, want 1", result["writeCount"])
	}
	if _, ok := result["splitWrites"]; ok {
		t.Fatal("a single-write publish must not be marked as split")
	}
}

// The document is created before any content is written, so a mid-sequence failure
// must name the created document instead of discarding its URL.
func TestPublishMarkdownReportsCreatedDocumentWhenAWriteFails(t *testing.T) {
	markdownPath := writeLargeMarkdown(t, blockEntryWriteBudget+250)
	fake := &splitPublishServer{failAtWrite: 2}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	_, err := PublishMarkdown(Config{
		MarkdownPath: markdownPath,
		BaseURL:      server.URL,
		CookiesPath:  writeFixtureCookies(t),
		SpaceAPI:     server.URL,
		Apply:        true,
	})
	if err == nil {
		t.Fatal("expected the failed write to surface an error")
	}
	message := err.Error()
	if !strings.Contains(message, "doxrzCreatedPage") {
		t.Fatalf("error must name the created document URL, got %q", message)
	}
	if !strings.Contains(message, "is incomplete") {
		t.Fatalf("error must say the document is incomplete, got %q", message)
	}
	if !strings.Contains(message, "after 1 of 2 writes") {
		t.Fatalf("error must report write progress, got %q", message)
	}
	if !strings.Contains(message, "delete it in the web UI") {
		t.Fatalf("error must say cleanup is manual, got %q", message)
	}
}

// A rejected oversized write must name the measured limit rather than leaving the
// reader to bisect for it.
func TestWriteBlocksExplainsAnOversizedRejection(t *testing.T) {
	fake := &splitPublishServer{}
	server := httptest.NewServer(fake.handler(t))
	defer server.Close()

	session, err := newPublishSession(Config{
		CookiesPath: writeFixtureCookies(t),
		SpaceAPI:    server.URL,
	}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	changeMap := map[string]any{"doxrzCreatedPage": map[string]any{"id": "doxrzCreatedPage"}}
	for index := 0; index < maxChangeEntriesPerWrite; index++ {
		id := fmt.Sprintf("block_%d", index)
		changeMap[id] = map[string]any{"id": id}
	}
	err = session.writeBlocks("doxrzCreatedPage", "member", changeMap, server.URL+"/docx/doxrzCreatedPage")
	if err == nil {
		t.Fatal("expected the oversized write to be rejected")
	}
	message := err.Error()
	if !strings.Contains(message, "3500") {
		t.Fatalf("error must name the measured limit, got %q", message)
	}
	if !strings.Contains(message, "count limit, not a byte limit") {
		t.Fatalf("error must state the cause, got %q", message)
	}
}
