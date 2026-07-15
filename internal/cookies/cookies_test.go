package cookies

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriteExportWritesPrivateCookieJSONAndDoesNotLeakValues(t *testing.T) {
	output := filepath.Join(t.TempDir(), "cookies.json")

	payload, err := writeExport("fixture-provider", output, []browserCookie{
		{Name: "_csrf_token", Value: "dummy-csrf", Domain: ".example.test", Path: "/", Secure: true},
		{Name: "session", Value: "dummy-session", Domain: ".example.test", Path: "/", Secure: true},
	})
	if err != nil {
		t.Fatalf("writeExport returned error: %v", err)
	}

	serialized := fmt.Sprint(payload)
	if strings.Contains(serialized, "dummy-csrf") || strings.Contains(serialized, "dummy-session") {
		t.Fatalf("payload leaked cookie values: %s", serialized)
	}
	if payload["cookieCount"] != 2 || payload["hasCsrf"] != true || payload["provider"] != "fixture-provider" {
		t.Fatalf("payload = %+v, want count=2 csrf=true provider=fixture-provider", payload)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(output)
		if err != nil {
			t.Fatalf("stat output: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("output mode = %#o, want 0600", mode)
		}
	}
	var cookies []browserCookie
	if err := readJSONFile(output, &cookies); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(cookies) != 2 || cookies[0].Name != "_csrf_token" || cookies[0].Value != "dummy-csrf" {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestWriteExportRejectsMissingNonEmptyCSRF(t *testing.T) {
	_, err := writeExport("fixture-provider", filepath.Join(t.TempDir(), "cookies.json"), []browserCookie{
		{Name: "_csrf_token", Value: "", Domain: ".example.test", Path: "/"},
		{Name: "session", Value: "dummy-session", Domain: ".example.test", Path: "/"},
	})

	if err == nil || !strings.Contains(err.Error(), "non-empty _csrf_token") {
		t.Fatalf("writeExport error = %v, want missing CSRF error", err)
	}
}

func TestResolveMacOSCookieDBPicksNewestProfileExplorerDB(t *testing.T) {
	appSupport := t.TempDir()
	oldDB := filepath.Join(appSupport, "aha", "users", "old", "profile_explorer", "Cookies")
	newDB := filepath.Join(appSupport, "aha", "users", "new", "profile_explorer", "Cookies")
	for _, path := range []string{oldDB, newDB} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("sqlite"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := os.Chtimes(oldDB, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatalf("chtimes old DB: %v", err)
	}
	if err := os.Chtimes(newDB, time.Unix(200, 0), time.Unix(200, 0)); err != nil {
		t.Fatalf("chtimes new DB: %v", err)
	}

	got, err := resolveMacOSCookieDB(ExportConfig{AppSupport: appSupport})
	if err != nil {
		t.Fatalf("resolveMacOSCookieDB returned error: %v", err)
	}
	if got != newDB {
		t.Fatalf("resolveMacOSCookieDB = %s, want newest %s", got, newDB)
	}
}

func TestExportMacOSReadsPlainRowsWithoutKeychainAndWritesSecretSafePayload(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "Cookies")
	createMacOSCookieDB(t, dbPath)
	output := filepath.Join(tmpDir, "cookies.json")

	payload, err := exportMacOS(ExportConfig{
		CookiesDB: dbPath,
		Output:    output,
		HostLike:  "%.example.test%",
	})
	if err != nil {
		t.Fatalf("exportMacOS returned error: %v", err)
	}
	if payload["provider"] != "macos-larkshell" || payload["cookieCount"] != 2 || payload["hasCsrf"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	if strings.Contains(fmt.Sprint(payload), "dummy-csrf") {
		t.Fatalf("payload leaked cookie value: %+v", payload)
	}
	var cookies []browserCookie
	if err := readJSONFile(output, &cookies); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(cookies) != 2 || cookies[0].Name != "_csrf_token" || cookies[0].Value != "dummy-csrf" {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestMacOSKeyFromKeychainOmitsAccountWhenUnset(t *testing.T) {
	original := commandOutput
	t.Cleanup(func() { commandOutput = original })
	var receivedName string
	var receivedArgs []string
	commandOutput = func(name string, args ...string) ([]byte, error) {
		receivedName = name
		receivedArgs = append([]string(nil), args...)
		return []byte("dummy-password\n"), nil
	}

	if _, err := macOSKeyFromKeychain("Suite App Safe Storage", ""); err != nil {
		t.Fatalf("macOSKeyFromKeychain returned error: %v", err)
	}
	if receivedName != "security" {
		t.Fatalf("command name = %s, want security", receivedName)
	}
	for _, arg := range receivedArgs {
		if arg == "-a" {
			t.Fatalf("command args included account flag without account: %v", receivedArgs)
		}
	}
}

func createMacOSCookieDB(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir DB dir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE cookies (
	host_key TEXT,
	name TEXT,
	value TEXT,
	encrypted_value BLOB,
	path TEXT,
	expires_utc INTEGER,
	is_secure INTEGER,
	is_httponly INTEGER,
	samesite INTEGER
)`)
	if err != nil {
		t.Fatalf("create cookies table: %v", err)
	}
	_, err = db.Exec(
		"INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		".example.test", "_csrf_token", "dummy-csrf", []byte{}, "/", 0, 1, 1, 0,
		".example.test", "session", "dummy-session", []byte{}, "/", 0, 1, 0, 1,
	)
	if err != nil {
		t.Fatalf("insert cookies: %v", err)
	}
}

func readJSONFile(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, target)
}
