package messenger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTitlesMatchNormalizesWhitespaceCaseAndPunctuation(t *testing.T) {
	tests := []struct {
		actual   string
		expected string
		match    bool
	}{
		{actual: " 示例 群聊 ", expected: "示例群聊", match: true},
		{actual: "Alice Zhang azhang1", expected: "alice zhang", match: true},
		{actual: "产品-研发(周会)", expected: "产品研发周会", match: true},
		{actual: "另一个群", expected: "示例群聊", match: false},
		{actual: "", expected: "示例群聊", match: false},
	}
	for _, test := range tests {
		if got := TitlesMatch(test.actual, test.expected); got != test.match {
			t.Fatalf("TitlesMatch(%q, %q) = %t, want %t", test.actual, test.expected, got, test.match)
		}
	}
}

func TestLoadBrowserCookiesParsesPlaywrightCookieJSONWithoutLeakingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := os.WriteFile(path, []byte(`[
		{"name":"_csrf_token","value":"secret-csrf","domain":".example.test","path":"/","secure":true,"httpOnly":true,"expires":1893456000,"sameSite":"Lax"},
		{"name":"session","value":"secret-session","domain":".example.test","path":"/"}
	]`), 0o600); err != nil {
		t.Fatalf("write cookies: %v", err)
	}

	cookies, err := LoadBrowserCookies(path)

	if err != nil {
		t.Fatalf("LoadBrowserCookies returned error: %v", err)
	}
	if len(cookies) != 2 {
		t.Fatalf("len(cookies) = %d, want 2", len(cookies))
	}
	if cookies[0].Name != "_csrf_token" || cookies[0].Value != "secret-csrf" || cookies[0].SameSite != "Lax" {
		t.Fatalf("first cookie = %+v", cookies[0])
	}

	invalidPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(`{"name":"_csrf_token","value":"secret-csrf"}`), 0o600); err != nil {
		t.Fatalf("write invalid cookies: %v", err)
	}
	_, err = LoadBrowserCookies(invalidPath)
	if err == nil {
		t.Fatal("LoadBrowserCookies accepted non-array JSON")
	}
	if strings.Contains(err.Error(), "secret-csrf") {
		t.Fatalf("LoadBrowserCookies leaked cookie value in error: %v", err)
	}
}
