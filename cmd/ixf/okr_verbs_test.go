package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// okrVerbServer serves a page with three objectives, O3 holding two KRs, and
// records the write calls it receives.
type okrVerbServer struct {
	events     []string
	deletedKRs []string
	createdKRs int
	finalKRs   []string
}

func (s *okrVerbServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/lgw/csrf_token":
			w.Header().Set("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
			_, _ = w.Write([]byte("{}"))
		case strings.HasSuffix(r.URL.Path, "/aggr_detail/"):
			s.events = append(s.events, "detail")
			writeTestJSON(t, w, s.detail())
		case strings.Contains(r.URL.Path, "/draft_v2/enable/"):
			s.events = append(s.events, "enable")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"draft_version": "7"}})
		case strings.Contains(r.URL.Path, "/draft_v2/kr/pos/"):
			s.events = append(s.events, "order")
			writeTestJSON(t, w, map[string]any{"code": 0})
		case strings.Contains(r.URL.Path, "/draft_v2/kr/") && r.Method == http.MethodDelete:
			s.events = append(s.events, "delete_kr")
			s.deletedKRs = append(s.deletedKRs, r.URL.Path)
			writeTestJSON(t, w, map[string]any{"code": 0})
		case strings.Contains(r.URL.Path, "/draft_v2/kr/") && r.Method == http.MethodPost:
			s.events = append(s.events, "create_kr")
			s.createdKRs++
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"kr_id": "made-kr"}})
		case strings.Contains(r.URL.Path, "/draft_v2/kr/") && r.Method == http.MethodPut:
			s.events = append(s.events, "kr_text")
			writeTestJSON(t, w, map[string]any{"code": 0})
		case strings.Contains(r.URL.Path, "/draft_v2/objective/") && r.Method == http.MethodDelete:
			s.events = append(s.events, "delete_objective")
			writeTestJSON(t, w, map[string]any{"code": 0})
		case strings.Contains(r.URL.Path, "/draft_v2/objective/") && r.Method == http.MethodPut:
			s.events = append(s.events, "objective_title")
			writeTestJSON(t, w, map[string]any{"code": 0})
		case strings.Contains(r.URL.Path, "/draft_v2/objective/"):
			s.events = append(s.events, "create_objective")
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"objective_id": "made-o"}})
		case strings.Contains(r.URL.Path, "/draft_v2/publish/"):
			s.events = append(s.events, "publish")
			writeTestJSON(t, w, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	}))
}

func (s *okrVerbServer) detail() map[string]any {
	krs := []any{
		okrKRFixture("kr-a", "KR A"),
		okrKRFixture("kr-b", "KR B"),
	}
	if s.finalKRs != nil {
		krs = []any{}
		for index, text := range s.finalKRs {
			krs = append(krs, okrKRFixture("kr-final-"+string(rune('a'+index)), text))
		}
	}
	return map[string]any{
		"code": 0,
		"okr_detail_data": map[string]any{
			"name": "2026 Q3",
			"objective_list": []any{
				map[string]any{"id": "o1", "name": okrNameFixture("O1"), "kr_list": []any{}},
				map[string]any{"id": "o2", "name": okrNameFixture("O2"), "kr_list": []any{}},
				map[string]any{"id": "o3", "name": okrNameFixture("O3"), "kr_list": krs},
			},
		},
	}
}

func okrNameFixture(text string) map[string]any {
	return map[string]any{"blocks": []any{map[string]any{"text": text}}}
}

func okrKRFixture(id string, text string) map[string]any {
	return map[string]any{"id": id, "content": okrNameFixture(text)}
}

func okrVerbURL(server *httptest.Server) string {
	return server.URL + "/okr/user/example/?okrId=example-okr"
}

// The removed command must name the verb replacing each old flag combination,
// because the callers most likely to reach it are running a cached older skill.
func TestCLIOKRWriteIsRemovedWithAMigrationMap(t *testing.T) {
	stdout, stderr, code := runCLITest(t, "okr", "write", "--url", "https://x.test", "--input", "f.json")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%q", code, stdout)
	}
	for _, expected := range []string{
		"was removed in 3.28.0",
		"ixf okr kr replace",
		"ixf okr objective create",
		"ixf okr kr add",
		"ixf okr objective delete",
		"ixf okr inspect",
	} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("migration message missing %q:\n%s", expected, stderr)
		}
	}
}

func TestCLIOKRKRReplaceDryRunReportsDiffAndBlocksApply(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	stdout, stderr, code := runCLITest(t,
		"okr", "kr", "replace",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR New",
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["destructive"] != true || payload["willWrite"] != false {
		t.Fatalf("payload = %+v", payload)
	}
	diff := payload["diff"].(map[string]any)
	if diff["krsToDelete"] != float64(2) || diff["resultingKrCount"] != float64(1) {
		t.Fatalf("diff = %+v", diff)
	}
	apply := payload["apply"].(map[string]any)
	if apply["blocked"] != true {
		t.Fatal("apply should be blocked without --confirm-kr-deletes")
	}
	required := apply["requiredFlags"].([]any)
	if len(required) != 1 || required[0] != "--confirm-kr-deletes 2" {
		t.Fatalf("requiredFlags = %+v, should name the exact count", required)
	}
	for _, event := range fake.events {
		if event != "detail" {
			t.Fatalf("dry-run performed %q; it must only read", event)
		}
	}
}

// A count that does not match the live state means the caller is working from a
// stale inspect, so the write is refused rather than applied.
func TestCLIOKRKRReplaceRefusesAWrongConfirmCount(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	_, stderr, code := runCLITest(t,
		"okr", "kr", "replace",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR New",
		"--confirm-kr-deletes", "1",
		"--apply",
	)
	if code == 0 {
		t.Fatal("a mismatched confirmation count must not apply")
	}
	if !strings.Contains(stderr, "does not match") {
		t.Fatalf("stderr = %q", stderr)
	}
	for _, event := range fake.events {
		if event == "delete_kr" || event == "create_kr" {
			t.Fatalf("refused command still wrote: %v", fake.events)
		}
	}
}

// Missing confirmation is a distinct case from a wrong one: the message must state
// the number to pass.
func TestCLIOKRKRReplaceRefusesWithoutConfirmation(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	_, stderr, code := runCLITest(t,
		"okr", "kr", "replace",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR New",
		"--apply",
	)
	if code == 0 {
		t.Fatal("apply without confirmation must be refused")
	}
	if !strings.Contains(stderr, "--confirm-kr-deletes 2") {
		t.Fatalf("stderr should name the count to pass, got %q", stderr)
	}
}

// The title guard is what makes a positional index safe.
func TestCLIOKRVerbsRefuseATitleMismatch(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	_, stderr, code := runCLITest(t,
		"okr", "kr", "add",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "Something Else",
		"--kr", "KR New",
		"--dry-run",
	)
	if code == 0 {
		t.Fatal("a title mismatch must refuse")
	}
	if !strings.Contains(stderr, "refusing to write") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLIOKRVerbsRequireAnExpectedTitle(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	_, stderr, code := runCLITest(t,
		"okr", "kr", "add",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--kr", "KR New",
		"--dry-run",
	)
	if code == 0 {
		t.Fatal("a missing --expect-title must refuse")
	}
	if !strings.Contains(stderr, "--expect-title is required") {
		t.Fatalf("stderr = %q", stderr)
	}
}

// kr add keeps what is there, which is the reason it exists as its own verb.
func TestCLIOKRKRAddReportsNoDeletions(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	stdout, stderr, code := runCLITest(t,
		"okr", "kr", "add",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR C",
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["destructive"] != false {
		t.Fatal("kr add must report destructive:false")
	}
	diff := payload["diff"].(map[string]any)
	if diff["krsToDelete"] != float64(0) || diff["resultingKrCount"] != float64(3) {
		t.Fatalf("diff = %+v", diff)
	}
}

// A KR already present is skipped rather than duplicated.
func TestCLIOKRKRAddSkipsKRsAlreadyPresent(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	stdout, _, code := runCLITest(t,
		"okr", "kr", "add",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR A",
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	payload := decodeCLIJSON(t, stdout)
	diff := payload["diff"].(map[string]any)
	if diff["krsToCreate"] != float64(0) {
		t.Fatalf("an existing KR should not be recreated: %+v", diff)
	}
	present := diff["alreadyPresent"].([]any)
	if len(present) != 1 || present[0] != "KR A" {
		t.Fatalf("alreadyPresent = %+v", present)
	}
}

// objective create refuses to target an existing objective, which is what removes
// the old create-or-replace ambiguity.
func TestCLIOKRObjectiveCreateRefusesAnExistingIndex(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	_, stderr, code := runCLITest(t,
		"okr", "objective", "create",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "2",
		"--title", "New O",
		"--kr", "KR 1",
		"--dry-run",
	)
	if code == 0 {
		t.Fatal("create must refuse an index that is not the append position")
	}
	if !strings.Contains(stderr, "appends at index 4") {
		t.Fatalf("stderr should name the append position, got %q", stderr)
	}
}

// kr delete is the only path to zero KRs, and a KR named but absent aborts the
// whole command rather than deleting the subset that matched.
func TestCLIOKRKRDeleteRefusesUnmatchedKRs(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	_, stderr, code := runCLITest(t,
		"okr", "kr", "delete",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR A",
		"--kr", "KR Nonexistent",
		"--confirm-kr-deletes", "2",
		"--apply",
	)
	if code == 0 {
		t.Fatal("an unmatched KR must abort the command")
	}
	if !strings.Contains(stderr, "Nothing was deleted") {
		t.Fatalf("stderr should say nothing was deleted, got %q", stderr)
	}
	for _, event := range fake.events {
		if event == "delete_kr" {
			t.Fatalf("refused delete still wrote: %v", fake.events)
		}
	}
}

func TestCLIOKRKRDeleteFlagsEmptyingAnObjective(t *testing.T) {
	fake := &okrVerbServer{}
	server := fake.start(t)
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeCLICookieFixture(t, cookiesPath)

	stdout, _, code := runCLITest(t,
		"okr", "kr", "delete",
		"--url", okrVerbURL(server),
		"--cookies", cookiesPath,
		"--csrf-url", server.URL+"/lgw/csrf_token",
		"--objective", "3",
		"--expect-title", "O3",
		"--kr", "KR A",
		"--kr", "KR B",
		"--dry-run",
	)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	payload := decodeCLIJSON(t, stdout)
	diff := payload["diff"].(map[string]any)
	if diff["emptiesObjective"] != true || diff["resultingKrCount"] != float64(0) {
		t.Fatalf("diff should flag that the objective ends up empty: %+v", diff)
	}
}

func TestCLIOKRVerbHelpExitsZero(t *testing.T) {
	for _, args := range [][]string{
		{"okr", "objective", "--help"},
		{"okr", "kr", "--help"},
		{"okr", "kr", "replace", "--help"},
		{"okr", "objective", "delete", "--help"},
	} {
		stdout, stderr, code := runCLITest(t, args...)
		if code != 0 {
			t.Fatalf("%v exit code = %d, stderr=%q", args, code, stderr)
		}
		if !strings.Contains(stdout, "usage: ixf okr") {
			t.Fatalf("%v stdout = %q", args, stdout)
		}
	}
}
