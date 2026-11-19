package cookies

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestResolveMacOSCookieCandidatesIncludesSupportedLiveProfilesAndExcludesClones(t *testing.T) {
	appSupport := t.TempDir()
	userRoot := filepath.Join(appSupport, "aha", "users", "user-a")
	rootDB := filepath.Join(userRoot, "profile_explorer", "Cookies")
	defaultDB := filepath.Join(userRoot, "profile_explorer", "Default", "Cookies")
	oldPartitionDB := filepath.Join(userRoot, "PartitionsV2", "openplatform_web_container_old", "Cookies")
	newPartitionDB := filepath.Join(userRoot, "PartitionsV2", "openplatform_web_container_new", "Cookies")
	cloneDB := filepath.Join(userRoot, "profile_explorer-playwright-search", "Default", "Cookies")
	for _, path := range []string{rootDB, defaultDB, oldPartitionDB, newPartitionDB, cloneDB} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("sqlite"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := os.Chtimes(oldPartitionDB, time.Unix(100, 0), time.Unix(100, 0)); err != nil {
		t.Fatalf("chtimes old partition: %v", err)
	}
	if err := os.Chtimes(newPartitionDB, time.Unix(200, 0), time.Unix(200, 0)); err != nil {
		t.Fatalf("chtimes new partition: %v", err)
	}

	got, err := resolveMacOSCookieCandidates(ExportConfig{AppSupport: appSupport})
	if err != nil {
		t.Fatalf("resolveMacOSCookieCandidates returned error: %v", err)
	}
	want := []macOSCookieCandidate{
		{Path: rootDB, SourceKind: "profile_explorer"},
		{Path: defaultDB, SourceKind: "profile_explorer_default"},
		{Path: newPartitionDB, SourceKind: "openplatform_web_container"},
		{Path: oldPartitionDB, SourceKind: "openplatform_web_container"},
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResolveMacOSCookieCandidatesKeepsExplicitDatabaseExact(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "Cookies")
	if err := os.WriteFile(explicit, []byte("sqlite"), 0o600); err != nil {
		t.Fatalf("write explicit DB: %v", err)
	}

	got, err := resolveMacOSCookieCandidates(ExportConfig{
		AppSupport: t.TempDir(),
		CookiesDB:  explicit,
	})
	if err != nil {
		t.Fatalf("resolveMacOSCookieCandidates returned error: %v", err)
	}
	want := []macOSCookieCandidate{{Path: explicit, SourceKind: "explicit"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("candidates = %+v, want %+v", got, want)
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

func TestExportMacOSFallsBackToUsablePartitionAndReportsStaleCSRF(t *testing.T) {
	original := commandOutput
	t.Cleanup(func() { commandOutput = original })
	commandOutput = func(_ string, _ ...string) ([]byte, error) {
		return []byte("fixture-keychain-password\n"), nil
	}

	appSupport := t.TempDir()
	userRoot := filepath.Join(appSupport, "aha", "users", "user-a")
	rootDB := filepath.Join(userRoot, "profile_explorer", "Cookies")
	defaultDB := filepath.Join(userRoot, "profile_explorer", "Default", "Cookies")
	partitionDB := filepath.Join(userRoot, "PartitionsV2", "openplatform_web_container_docs", "Cookies")
	createMacOSCookieDBWithRows(t, rootDB, []macOSCookieFixtureRow{
		{Host: ".example.test", Name: "session", Value: "root-session-secret"},
	})
	createMacOSCookieDBWithRows(t, defaultDB, []macOSCookieFixtureRow{
		{Host: ".example.test", Name: "_csrf_token", EncryptedValue: []byte("v10bad-encryption")},
	})
	createMacOSCookieDBWithRows(t, partitionDB, []macOSCookieFixtureRow{
		{
			Host:       ".example.test",
			Name:       "_csrf_token",
			Value:      "partition-csrf-secret",
			ExpiresUTC: chromiumExpiryFromUnix(time.Now().Add(-time.Hour).Unix()),
		},
		{Host: ".example.test", Name: "session", Value: "partition-session-secret"},
	})
	output := filepath.Join(t.TempDir(), "cookies.json")

	payload, err := exportMacOS(ExportConfig{
		AppSupport: appSupport,
		Output:     output,
		HostLike:   "%.example.test%",
	})
	if err != nil {
		t.Fatalf("exportMacOS returned error: %v", err)
	}
	if payload["sourceKind"] != "openplatform_web_container" || payload["candidateCount"] != 3 || payload["csrfExpired"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	candidateDiagnostics, ok := payload["candidates"].([]CandidateDiagnostic)
	if !ok || len(candidateDiagnostics) != 3 {
		t.Fatalf("candidate diagnostics = %#v, want three evaluated candidates", payload["candidates"])
	}
	if candidateDiagnostics[0].Rejection != "required_cookie_missing" ||
		candidateDiagnostics[1].Rejection != "decrypt_failed" ||
		candidateDiagnostics[2].SourceKind != "openplatform_web_container" ||
		candidateDiagnostics[2].Rejection != "" ||
		!candidateDiagnostics[2].HasCSRF {
		t.Fatalf("candidate diagnostics = %+v", candidateDiagnostics)
	}
	warnings, ok := payload["warnings"].([]string)
	if !ok || len(warnings) != 1 || !strings.Contains(warnings[0], "expired") {
		t.Fatalf("warnings = %#v, want one stale CSRF warning", payload["warnings"])
	}
	serialized := fmt.Sprint(payload)
	for _, secret := range []string{"root-session-secret", "partition-csrf-secret", "partition-session-secret"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("payload leaked %q: %s", secret, serialized)
		}
	}
	var exported []browserCookie
	if err := readJSONFile(output, &exported); err != nil {
		t.Fatalf("read exported cookies: %v", err)
	}
	if len(exported) != 2 || exported[0].Name != "_csrf_token" || exported[0].Value != "partition-csrf-secret" {
		t.Fatalf("exported cookies = %+v", exported)
	}
}

func TestExportMacOSExplicitDatabaseDoesNotFallBack(t *testing.T) {
	appSupport := t.TempDir()
	userRoot := filepath.Join(appSupport, "aha", "users", "user-a")
	explicitDB := filepath.Join(userRoot, "profile_explorer", "Cookies")
	partitionDB := filepath.Join(userRoot, "PartitionsV2", "openplatform_web_container_docs", "Cookies")
	createMacOSCookieDBWithRows(t, explicitDB, []macOSCookieFixtureRow{
		{Host: ".example.test", Name: "session", Value: "explicit-session-secret"},
	})
	createMacOSCookieDBWithRows(t, partitionDB, []macOSCookieFixtureRow{
		{Host: ".example.test", Name: "_csrf_token", Value: "fallback-csrf-secret"},
	})

	_, err := exportMacOS(ExportConfig{
		AppSupport: appSupport,
		CookiesDB:  explicitDB,
		Output:     filepath.Join(t.TempDir(), "cookies.json"),
		HostLike:   "%.example.test%",
	})
	if err == nil {
		t.Fatal("exportMacOS returned nil error, want explicit source failure")
	}
	var exportErr *ExportError
	if !errors.As(err, &exportErr) {
		t.Fatalf("error type = %T, want *ExportError", err)
	}
	if exportErr.Reason != "required_cookie_missing" || len(exportErr.Candidates) != 1 || exportErr.Candidates[0].SourceKind != "explicit" {
		t.Fatalf("export error = %+v", exportErr)
	}
	serialized := fmt.Sprint(exportErr)
	for _, secret := range []string{"explicit-session-secret", "fallback-csrf-secret"} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("error leaked %q: %s", secret, serialized)
		}
	}
}

func TestExportMacOSFailureAggregatesSecretSafeCandidateDiagnostics(t *testing.T) {
	appSupport := t.TempDir()
	userRoot := filepath.Join(appSupport, "aha", "users", "user-a")
	rootDB := filepath.Join(userRoot, "profile_explorer", "Cookies")
	defaultDB := filepath.Join(userRoot, "profile_explorer", "Default", "Cookies")
	createMacOSCookieDBWithRows(t, rootDB, []macOSCookieFixtureRow{
		{Host: ".example.test", Name: "session", Value: "root-session-secret"},
	})
	if err := os.MkdirAll(filepath.Dir(defaultDB), 0o755); err != nil {
		t.Fatalf("mkdir default DB: %v", err)
	}
	if err := os.WriteFile(defaultDB, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("write invalid default DB: %v", err)
	}

	_, err := exportMacOS(ExportConfig{
		AppSupport: appSupport,
		Output:     filepath.Join(t.TempDir(), "cookies.json"),
		HostLike:   "%.example.test%",
	})
	if err == nil {
		t.Fatal("exportMacOS returned nil error, want aggregate failure")
	}
	var exportErr *ExportError
	if !errors.As(err, &exportErr) {
		t.Fatalf("error type = %T, want *ExportError", err)
	}
	if exportErr.Reason != "no_usable_cookie_source" || len(exportErr.Candidates) != 2 {
		t.Fatalf("export error = %+v", exportErr)
	}
	if exportErr.Candidates[0].Rejection != "required_cookie_missing" || exportErr.Candidates[1].Rejection != "read_failed" {
		t.Fatalf("candidate diagnostics = %+v", exportErr.Candidates)
	}
	if strings.Contains(fmt.Sprint(exportErr), "root-session-secret") {
		t.Fatalf("error leaked cookie value: %+v", exportErr)
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

func TestResolveWindowsCookieDBAndLocalState(t *testing.T) {
	tmpDir := t.TempDir()
	appData := filepath.Join(tmpDir, "Roaming")
	userData := filepath.Join(appData, "LarkShell", "User Data")
	dbPath := filepath.Join(userData, "Default", "Network", "Cookies")
	localState := filepath.Join(userData, "Local State")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir DB dir: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("sqlite"), 0o600); err != nil {
		t.Fatalf("write DB: %v", err)
	}
	if err := os.WriteFile(localState, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write Local State: %v", err)
	}

	resolvedDB, err := resolveWindowsCookieDB(ExportConfig{AppData: appData})
	if err != nil {
		t.Fatalf("resolveWindowsCookieDB returned error: %v", err)
	}
	if resolvedDB != dbPath {
		t.Fatalf("resolveWindowsCookieDB = %s, want %s", resolvedDB, dbPath)
	}
	resolvedState, err := resolveWindowsLocalState(ExportConfig{}, dbPath)
	if err != nil {
		t.Fatalf("resolveWindowsLocalState returned error: %v", err)
	}
	if resolvedState != localState {
		t.Fatalf("resolveWindowsLocalState = %s, want %s", resolvedState, localState)
	}
}

func TestLoadWindowsMasterKeyUnwrapsDPAPIKeyAndNormalizesInvalidJSON(t *testing.T) {
	original := dpapiUnprotectBytes
	t.Cleanup(func() { dpapiUnprotectBytes = original })
	wrapped := []byte("wrapped-local-state-key")
	unwrapped := []byte("0123456789abcdef0123456789abcdef")
	dpapiUnprotectBytes = func(encrypted []byte) ([]byte, error) {
		if string(encrypted) != string(wrapped) {
			t.Fatalf("dpapi input = %q, want %q", encrypted, wrapped)
		}
		return unwrapped, nil
	}

	localState := filepath.Join(t.TempDir(), "Local State")
	writeLocalState(t, localState, base64.StdEncoding.EncodeToString(append([]byte("DPAPI"), wrapped...)))

	key, err := loadWindowsMasterKey(localState)
	if err != nil {
		t.Fatalf("loadWindowsMasterKey returned error: %v", err)
	}
	if string(key) != string(unwrapped) {
		t.Fatalf("key = %q, want %q", key, unwrapped)
	}

	invalid := filepath.Join(t.TempDir(), "Invalid Local State")
	if err := os.WriteFile(invalid, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write invalid Local State: %v", err)
	}
	if _, err := loadWindowsMasterKey(invalid); err == nil || !strings.Contains(err.Error(), "Local State is invalid") {
		t.Fatalf("invalid Local State error = %v", err)
	}
}

func TestDecryptWindowsCookieValueHandlesAESGCMDPAPIAndErrors(t *testing.T) {
	original := dpapiUnprotectBytes
	t.Cleanup(func() { dpapiUnprotectBytes = original })
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	modern := encryptChromiumCookie(t, masterKey, []byte("synthetic-csrf"))

	value, err := decryptWindowsCookieValue(modern, masterKey)
	if err != nil {
		t.Fatalf("decryptWindowsCookieValue modern error = %v", err)
	}
	if value != "synthetic-csrf" {
		t.Fatalf("modern value = %q", value)
	}

	dpapiUnprotectBytes = func(encrypted []byte) ([]byte, error) {
		if string(encrypted) != "legacy-encrypted" {
			t.Fatalf("dpapi input = %q", encrypted)
		}
		return []byte("legacy-cookie"), nil
	}
	value, err = decryptWindowsCookieValue([]byte("legacy-encrypted"), masterKey)
	if err != nil {
		t.Fatalf("decryptWindowsCookieValue legacy error = %v", err)
	}
	if value != "legacy-cookie" {
		t.Fatalf("legacy value = %q", value)
	}

	if _, err := decryptWindowsCookieValue(modern[:len(modern)-1], masterKey); err == nil || !strings.Contains(err.Error(), "could not decrypt Windows Chromium cookie value") {
		t.Fatalf("invalid AES-GCM error = %v", err)
	}
	dpapiUnprotectBytes = func(_ []byte) ([]byte, error) {
		return []byte{0xff}, nil
	}
	if _, err := decryptWindowsCookieValue([]byte("legacy-encrypted"), masterKey); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
	dpapiUnprotectBytes = func(_ []byte) ([]byte, error) {
		return nil, fmt.Errorf("fixture-dpapi-failure")
	}
	if _, err := decryptWindowsCookieValue([]byte("legacy-encrypted"), masterKey); err == nil || !strings.Contains(err.Error(), "could not decrypt Windows cookie value") {
		t.Fatalf("DPAPI failure error = %v", err)
	}
}

func TestExportWindowsDecryptsModernCookieWithLocalStateAndInjectedDPAPI(t *testing.T) {
	original := dpapiUnprotectBytes
	t.Cleanup(func() { dpapiUnprotectBytes = original })
	masterKey := []byte("0123456789abcdef0123456789abcdef")
	wrapped := []byte("wrapped-local-state-key")
	tmpDir := t.TempDir()
	userData := filepath.Join(tmpDir, "LarkShell", "User Data")
	localState := filepath.Join(userData, "Local State")
	dbPath := filepath.Join(userData, "Default", "Network", "Cookies")
	output := filepath.Join(tmpDir, "cookies.json")
	writeLocalState(t, localState, base64.StdEncoding.EncodeToString(append([]byte("DPAPI"), wrapped...)))
	createWindowsCookieDB(t, dbPath, []windowsCookieFixtureRow{
		{Host: ".example.test", Name: "_csrf_token", Value: "", EncryptedValue: encryptChromiumCookie(t, masterKey, []byte("csrf")), Path: "/", IsSecure: true},
	})
	dpapiUnprotectBytes = func(encrypted []byte) ([]byte, error) {
		if string(encrypted) != string(wrapped) {
			t.Fatalf("dpapi input = %q, want wrapped key", encrypted)
		}
		return masterKey, nil
	}

	payload, err := exportWindows(ExportConfig{
		CookiesDB:  dbPath,
		LocalState: localState,
		Output:     output,
		HostLike:   "%.example.test%",
	})
	if err != nil {
		t.Fatalf("exportWindows returned error: %v", err)
	}
	if payload["provider"] != "windows-larkshell" || payload["cookieCount"] != 1 || payload["hasCsrf"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	var cookies []browserCookie
	if err := readJSONFile(output, &cookies); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(cookies) != 1 || cookies[0].Value != "csrf" {
		t.Fatalf("cookies = %+v", cookies)
	}
}

type macOSCookieFixtureRow struct {
	Host           string
	Name           string
	Value          string
	EncryptedValue []byte
	Path           string
	ExpiresUTC     int64
	IsSecure       bool
	IsHTTPOnly     bool
	SameSite       int64
}

func createMacOSCookieDB(t *testing.T, path string) {
	createMacOSCookieDBWithRows(t, path, []macOSCookieFixtureRow{
		{Host: ".example.test", Name: "_csrf_token", Value: "dummy-csrf", Path: "/", IsSecure: true, IsHTTPOnly: true},
		{Host: ".example.test", Name: "session", Value: "dummy-session", Path: "/", IsSecure: true, SameSite: 1},
	})
}

func createMacOSCookieDBWithRows(t *testing.T, path string, rows []macOSCookieFixtureRow) {
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
	for _, row := range rows {
		secure := 0
		if row.IsSecure {
			secure = 1
		}
		httpOnly := 0
		if row.IsHTTPOnly {
			httpOnly = 1
		}
		pathValue := row.Path
		if pathValue == "" {
			pathValue = "/"
		}
		_, err = db.Exec(
			"INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			row.Host, row.Name, row.Value, row.EncryptedValue, pathValue, row.ExpiresUTC, secure, httpOnly, row.SameSite,
		)
		if err != nil {
			t.Fatalf("insert cookie: %v", err)
		}
	}
}

func chromiumExpiryFromUnix(unix int64) int64 {
	return (unix + 11644473600) * 1_000_000
}

type windowsCookieFixtureRow struct {
	Host           string
	Name           string
	Value          string
	EncryptedValue []byte
	Path           string
	IsSecure       bool
}

func createWindowsCookieDB(t *testing.T, path string, rows []windowsCookieFixtureRow) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir Windows DB dir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open Windows DB: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE cookies (
	host_key TEXT,
	name TEXT,
	value TEXT,
	encrypted_value BLOB,
	path TEXT,
	is_secure INTEGER
)`)
	if err != nil {
		t.Fatalf("create Windows cookies table: %v", err)
	}
	for _, row := range rows {
		secure := 0
		if row.IsSecure {
			secure = 1
		}
		_, err = db.Exec(
			"INSERT INTO cookies VALUES (?, ?, ?, ?, ?, ?)",
			row.Host,
			row.Name,
			row.Value,
			row.EncryptedValue,
			row.Path,
			secure,
		)
		if err != nil {
			t.Fatalf("insert Windows cookie: %v", err)
		}
	}
}

func writeLocalState(t *testing.T, path string, encryptedKey string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir Local State dir: %v", err)
	}
	content, err := json.Marshal(map[string]any{
		"os_crypt": map[string]any{"encrypted_key": encryptedKey},
	})
	if err != nil {
		t.Fatalf("marshal Local State: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write Local State: %v", err)
	}
}

func encryptChromiumCookie(t *testing.T, key []byte, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new AES cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new GCM: %v", err)
	}
	nonce := []byte("nonce-123456")
	if len(nonce) != gcm.NonceSize() {
		t.Fatalf("nonce size = %d, want %d", len(nonce), gcm.NonceSize())
	}
	return append(append([]byte("v10"), nonce...), gcm.Seal(nil, nonce, plaintext, nil)...)
}

func readJSONFile(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, target)
}
