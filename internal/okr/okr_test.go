package okr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectURLRecognizesOKRPagesOnly(t *testing.T) {
	if !DetectURL("https://example.xfchat.iflytek.com/okr/user/owner-fixture/?okrId=okr-fixture-200&type=leader") {
		t.Fatal("expected OKR URL to be detected")
	}
	if DetectURL("https://example.xfchat.iflytek.com/docx/doxfixture") {
		t.Fatal("expected docx URL not to be detected as OKR")
	}
}

func TestReadUsesLGWCSRFAndOKRIDQueryParamAndRendersMarkdown(t *testing.T) {
	var events []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lgw/csrf_token":
			events = append(events, "csrf")
			if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "session=session-fixture") {
				t.Fatalf("csrf Cookie header = %q, want session fixture", cookie)
			}
			w.Header().Set("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
			_, _ = w.Write([]byte("{}"))
		case "/okrx/api/okr/owner/aggr_detail/":
			events = append(events, "detail")
			if got := r.URL.Query().Get("okr_id"); got != "okr-fixture-200" {
				t.Fatalf("okr_id query = %q, want okr-fixture-200", got)
			}
			if got := r.URL.Query().Get("okrId"); got != "" {
				t.Fatalf("okrId query = %q, want empty legacy alias", got)
			}
			if got := r.Header.Get("x-lgw-csrf-token"); got != "lgw-fixture" {
				t.Fatalf("x-lgw-csrf-token = %q, want lgw-fixture", got)
			}
			writeOKRJSON(t, w, map[string]any{
				"code": 0,
				"okr_detail_data": map[string]any{
					"name": "2026 年 7 月 - 9 月",
					"owner_info": map[string]any{
						"user_info": map[string]any{
							"locale_names": map[string]any{"zh": "Fixture Owner"},
						},
					},
					"objective_list": []any{
						map[string]any{
							"id":   "o1",
							"name": map[string]any{"blocks": []any{map[string]any{"text": "支撑计算平台规模化落地"}}},
							"kr_list": []any{
								map[string]any{
									"id":            "kr1",
									"content":       map[string]any{"blocks": []any{map[string]any{"text": "SAE 生产应用数提升到 8000"}}},
									"progress_rate": map[string]any{"percent": 20},
								},
								map[string]any{
									"id": "kr2",
									"content_v2": map[string]any{
										"0": map[string]any{
											"ops": []any{map[string]any{"insert": "KubeVirt 云主机提升到 500+\n"}},
										},
									},
								},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeOKRCookieFixture(t, cookiesPath)

	source := server.URL + "/okr/user/owner-fixture/?lang=zh-CN&okrId=okr-fixture-200&type=leader"
	body, err := Read(ReadConfig{
		Source:      source,
		CookiesPath: cookiesPath,
		CSRFURL:     server.URL + "/lgw/csrf_token",
	})
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	want := "# OKR - Fixture Owner - 2026 年 7 月 - 9 月\n\n" +
		"[okr id=okr-fixture-200 objectives=1]\n\n" +
		"## O1 支撑计算平台规模化落地\n\n" +
		"- KR1: SAE 生产应用数提升到 8000 _(progress: 20%)_\n" +
		"- KR2: KubeVirt 云主机提升到 500+\n"
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if strings.Join(events, ",") != "csrf,detail" {
		t.Fatalf("events = %#v, want csrf then detail", events)
	}
}

func TestReadNonzeroResponseDoesNotExposePrivatePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lgw/csrf_token":
			w.Header().Set("Set-Cookie", "lgw_csrf_token=lgw-fixture; Path=/")
			_, _ = w.Write([]byte("{}"))
		case "/okrx/api/okr/owner/aggr_detail/":
			writeOKRJSON(t, w, map[string]any{
				"code":    403,
				"message": "private failure",
				"okr_detail_data": map[string]any{
					"objective_list": []any{map[string]any{"name": "secret objective"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cookiesPath := filepath.Join(t.TempDir(), "cookies.json")
	writeOKRCookieFixture(t, cookiesPath)

	_, err := Read(ReadConfig{
		Source:      server.URL + "/okr/user/owner-fixture/?okrId=okr-fixture-200",
		CookiesPath: cookiesPath,
		CSRFURL:     server.URL + "/lgw/csrf_token",
	})
	if err == nil {
		t.Fatal("expected read error")
	}
	message := err.Error()
	if message != "OKR aggr_detail failed with code 403" {
		t.Fatalf("error = %q, want safe status-only message", message)
	}
	if strings.Contains(message, "private failure") || strings.Contains(message, "secret objective") {
		t.Fatalf("error leaked private payload: %q", message)
	}
}

func writeOKRCookieFixture(t *testing.T, path string) {
	t.Helper()
	content, err := json.Marshal([]map[string]string{
		{"name": "session", "value": "session-fixture"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeOKRJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}
