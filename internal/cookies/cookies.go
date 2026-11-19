package cookies

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

const (
	DefaultAppSupport      = "~/Library/Application Support/LarkShell-ka-kaahyz17"
	DefaultHostLike        = "%xfchat.iflytek.com%"
	DefaultKeychainService = "Suite App Safe Storage"
)

type ExportConfig struct {
	Provider        string
	Output          string
	AppSupport      string
	CookiesDB       string
	HostLike        string
	KeychainService string
	KeychainAccount string
	AppData         string
	LocalState      string
}

var commandOutput = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

var dpapiUnprotectBytes = dpapiUnprotect

type cookieRow struct {
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

type browserCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
}

type macOSCookieCandidate struct {
	Path       string
	SourceKind string
}

type CandidateDiagnostic struct {
	SourceKind  string `json:"sourceKind"`
	CookieCount int    `json:"cookieCount"`
	HasCSRF     bool   `json:"hasCsrf"`
	CSRFExpired bool   `json:"csrfExpired"`
	Rejection   string `json:"rejection,omitempty"`
}

type ExportError struct {
	Reason     string                `json:"reason"`
	Candidates []CandidateDiagnostic `json:"candidates"`
}

func (e *ExportError) Error() string {
	switch e.Reason {
	case "cookie_source_not_found":
		return "no supported desktop cookie source was found"
	case "required_cookie_missing":
		return "cookie source does not contain a non-empty _csrf_token"
	case "read_failed":
		return "cookie source could not be read"
	case "decrypt_failed":
		return "cookie source could not be decrypted"
	default:
		return "no supported desktop cookie source produced a usable non-empty _csrf_token"
	}
}

func Export(config ExportConfig) (map[string]any, error) {
	provider := strings.TrimSpace(config.Provider)
	if provider == "" || provider == "auto" {
		switch runtime.GOOS {
		case "darwin":
			provider = "macos-larkshell"
		case "windows":
			provider = "windows-larkshell"
		default:
			return nil, fmt.Errorf("automatic cookie export supports macOS and Windows")
		}
	}
	switch provider {
	case "macos-larkshell":
		return exportMacOS(config)
	case "windows-larkshell":
		return exportWindows(config)
	default:
		return nil, fmt.Errorf("unsupported cookie provider: %s", config.Provider)
	}
}

func exportMacOS(config ExportConfig) (map[string]any, error) {
	candidates, err := resolveMacOSCookieCandidates(config)
	if err != nil {
		return nil, &ExportError{Reason: "cookie_source_not_found", Candidates: []CandidateDiagnostic{}}
	}
	hostLike := valueOrDefault(config.HostLike, DefaultHostLike)
	var key []byte
	var keyErr error
	diagnostics := make([]CandidateDiagnostic, 0, len(candidates))
	for _, candidate := range candidates {
		rows, readErr := readMacOSRows(candidate.Path, hostLike)
		if readErr != nil {
			diagnostics = append(diagnostics, CandidateDiagnostic{
				SourceKind: candidate.SourceKind,
				Rejection:  "read_failed",
			})
			continue
		}
		needsDecrypt := false
		for _, row := range rows {
			if row.Value == "" && len(row.EncryptedValue) > 0 {
				needsDecrypt = true
				break
			}
		}
		if needsDecrypt && key == nil && keyErr == nil {
			service := valueOrDefault(config.KeychainService, DefaultKeychainService)
			key, keyErr = macOSKeyFromKeychain(service, config.KeychainAccount)
		}
		if needsDecrypt && keyErr != nil {
			diagnostics = append(diagnostics, CandidateDiagnostic{
				SourceKind:  candidate.SourceKind,
				CookieCount: len(rows),
				Rejection:   "decrypt_failed",
			})
			continue
		}
		cookies, decryptErr := macOSBrowserCookies(rows, key)
		if decryptErr != nil {
			diagnostics = append(diagnostics, CandidateDiagnostic{
				SourceKind:  candidate.SourceKind,
				CookieCount: len(rows),
				Rejection:   "decrypt_failed",
			})
			continue
		}
		hasCSRF, csrfExpired := csrfStatus(cookies, time.Now())
		diagnostic := CandidateDiagnostic{
			SourceKind:  candidate.SourceKind,
			CookieCount: len(cookies),
			HasCSRF:     hasCSRF,
			CSRFExpired: csrfExpired,
		}
		if !hasCSRF {
			diagnostic.Rejection = "required_cookie_missing"
			diagnostics = append(diagnostics, diagnostic)
			continue
		}
		payload, writeErr := writeExport("macos-larkshell", config.Output, cookies)
		if writeErr != nil {
			return nil, writeErr
		}
		payload["sourceKind"] = candidate.SourceKind
		payload["candidateCount"] = len(candidates)
		payload["csrfExpired"] = csrfExpired
		if csrfExpired {
			payload["warnings"] = []string{
				"selected _csrf_token is expired according to Chromium metadata; the remaining session cookies may still be accepted",
			}
		}
		return payload, nil
	}
	reason := "no_usable_cookie_source"
	if len(candidates) == 1 && candidates[0].SourceKind == "explicit" && len(diagnostics) == 1 {
		reason = diagnostics[0].Rejection
	}
	return nil, &ExportError{Reason: reason, Candidates: diagnostics}
}

func macOSBrowserCookies(rows []cookieRow, key []byte) ([]browserCookie, error) {
	cookies := make([]browserCookie, 0, len(rows))
	for _, row := range rows {
		value := row.Value
		if value == "" && len(row.EncryptedValue) > 0 {
			var err error
			value, err = decryptMacOSCookieValue(row.Host, row.EncryptedValue, key)
			if err != nil {
				return nil, err
			}
		}
		cookie := browserCookie{
			Name:     row.Name,
			Value:    value,
			Domain:   row.Host,
			Path:     valueOrDefault(row.Path, "/"),
			Secure:   row.IsSecure,
			HTTPOnly: row.IsHTTPOnly,
		}
		if row.ExpiresUTC != 0 {
			cookie.Expires = chromiumExpiresToUnix(row.ExpiresUTC)
		}
		if sameSite := sameSiteName(row.SameSite); sameSite != "" {
			cookie.SameSite = sameSite
		}
		cookies = append(cookies, cookie)
	}
	return cookies, nil
}

func csrfStatus(cookies []browserCookie, now time.Time) (bool, bool) {
	hasCSRF := false
	allExpired := true
	for _, cookie := range cookies {
		if cookie.Name != "_csrf_token" || cookie.Value == "" {
			continue
		}
		hasCSRF = true
		if cookie.Expires == 0 || cookie.Expires > now.Unix() {
			allExpired = false
		}
	}
	return hasCSRF, hasCSRF && allExpired
}

func exportWindows(config ExportConfig) (map[string]any, error) {
	dbPath, err := resolveWindowsCookieDB(config)
	if err != nil {
		return nil, err
	}
	hostLike := valueOrDefault(config.HostLike, DefaultHostLike)
	rows, err := readWindowsRows(dbPath, hostLike)
	if err != nil {
		return nil, err
	}
	var masterKey []byte
	cookies := make([]browserCookie, 0, len(rows))
	for _, row := range rows {
		value := row.Value
		if value == "" && len(row.EncryptedValue) > 0 {
			if isModernChromiumCookie(row.EncryptedValue) && masterKey == nil {
				statePath, err := resolveWindowsLocalState(config, dbPath)
				if err != nil {
					return nil, err
				}
				masterKey, err = loadWindowsMasterKey(statePath)
				if err != nil {
					return nil, err
				}
			}
			value, err = decryptWindowsCookieValue(row.EncryptedValue, masterKey)
			if err != nil {
				return nil, err
			}
		}
		cookies = append(cookies, browserCookie{
			Name:   row.Name,
			Value:  value,
			Domain: row.Host,
			Path:   valueOrDefault(row.Path, "/"),
			Secure: row.IsSecure,
		})
	}
	return writeExport("windows-larkshell", config.Output, cookies)
}

func resolveMacOSCookieDB(config ExportConfig) (string, error) {
	candidates, err := resolveMacOSCookieCandidates(config)
	if err != nil {
		return "", err
	}
	return candidates[0].Path, nil
}

func resolveMacOSCookieCandidates(config ExportConfig) ([]macOSCookieCandidate, error) {
	if strings.TrimSpace(config.CookiesDB) != "" {
		return []macOSCookieCandidate{{
			Path:       expandUser(config.CookiesDB),
			SourceKind: "explicit",
		}}, nil
	}
	appSupport := expandUser(valueOrDefault(config.AppSupport, DefaultAppSupport))
	patterns := []struct {
		pattern    string
		sourceKind string
	}{
		{filepath.Join(appSupport, "aha", "users", "*", "profile_explorer", "Cookies"), "profile_explorer"},
		{filepath.Join(appSupport, "aha", "users", "*", "profile_explorer", "Default", "Cookies"), "profile_explorer_default"},
		{filepath.Join(appSupport, "aha", "users", "*", "PartitionsV2", "openplatform_web_container_*", "Cookies"), "openplatform_web_container"},
	}
	var result []macOSCookieCandidate
	for _, item := range patterns {
		paths, err := filepath.Glob(item.pattern)
		if err != nil {
			return nil, err
		}
		sort.Slice(paths, func(i, j int) bool {
			left, leftErr := os.Stat(paths[i])
			right, rightErr := os.Stat(paths[j])
			if leftErr != nil || rightErr != nil || left.ModTime().Equal(right.ModTime()) {
				return paths[i] < paths[j]
			}
			return left.ModTime().After(right.ModTime())
		})
		for _, path := range paths {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				result = append(result, macOSCookieCandidate{Path: path, SourceKind: item.sourceKind})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no supported Cookies database found under %s", appSupport)
	}
	return result, nil
}

func resolveWindowsCookieDB(config ExportConfig) (string, error) {
	if strings.TrimSpace(config.CookiesDB) != "" {
		path := expandUser(config.CookiesDB)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("Windows LarkShell cookie DB not found: %s", path)
		}
		return path, nil
	}
	appData := strings.TrimSpace(config.AppData)
	if appData == "" {
		appData = os.Getenv("APPDATA")
	}
	if appData == "" {
		return "", fmt.Errorf("Windows LarkShell cookie DB not found: APPDATA is not set")
	}
	root := expandUser(appData)
	candidates := []string{
		filepath.Join(root, "LarkShell", "User Data", "Default", "Network", "Cookies"),
		filepath.Join(root, "LarkShell", "User Data", "Default", "Cookies"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Windows LarkShell cookie DB not found under APPDATA")
}

func resolveWindowsLocalState(config ExportConfig, dbPath string) (string, error) {
	if strings.TrimSpace(config.LocalState) != "" {
		path := expandUser(config.LocalState)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("Windows LarkShell Local State not found: %s", path)
		}
		return path, nil
	}
	candidates := []string{
		filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(dbPath))), "Local State"),
		filepath.Join(filepath.Dir(filepath.Dir(dbPath)), "Local State"),
	}
	appData := strings.TrimSpace(config.AppData)
	if appData == "" {
		appData = os.Getenv("APPDATA")
	}
	if appData != "" {
		candidates = append(candidates, filepath.Join(expandUser(appData), "LarkShell", "User Data", "Local State"))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Windows LarkShell Local State not found")
}

func readMacOSRows(dbPath string, hostLike string) ([]cookieRow, error) {
	db, err := openSQLiteReadonly(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
SELECT host_key, name, value, encrypted_value, path, expires_utc, is_secure, is_httponly, samesite
FROM cookies
WHERE host_key LIKE ?
ORDER BY host_key, name`, hostLike)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []cookieRow
	for rows.Next() {
		var row cookieRow
		var secure, httpOnly int
		if err := rows.Scan(&row.Host, &row.Name, &row.Value, &row.EncryptedValue, &row.Path, &row.ExpiresUTC, &secure, &httpOnly, &row.SameSite); err != nil {
			return nil, err
		}
		row.IsSecure = secure != 0
		row.IsHTTPOnly = httpOnly != 0
		result = append(result, row)
	}
	return result, rows.Err()
}

func readWindowsRows(dbPath string, hostLike string) ([]cookieRow, error) {
	db, err := openSQLiteReadonly(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
SELECT host_key, name, value, encrypted_value, path, is_secure
FROM cookies
WHERE host_key LIKE ?
ORDER BY host_key, name`, hostLike)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []cookieRow
	for rows.Next() {
		var row cookieRow
		var secure int
		if err := rows.Scan(&row.Host, &row.Name, &row.Value, &row.EncryptedValue, &row.Path, &secure); err != nil {
			return nil, err
		}
		row.IsSecure = secure != 0
		result = append(result, row)
	}
	return result, rows.Err()
}

func openSQLiteReadonly(path string) (*sql.DB, error) {
	uri := "file:" + url.PathEscape(path) + "?mode=ro"
	return sql.Open("sqlite", uri)
}

func writeExport(provider string, output string, cookies []browserCookie) (map[string]any, error) {
	hasCsrf := false
	for _, cookie := range cookies {
		if cookie.Name == "_csrf_token" && cookie.Value != "" {
			hasCsrf = true
			break
		}
	}
	if !hasCsrf {
		return nil, fmt.Errorf("exported cookies do not contain a non-empty _csrf_token")
	}
	outPath, err := writeCookieJSON(output, cookies)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":          true,
		"provider":    provider,
		"cookieCount": len(cookies),
		"hasCsrf":     hasCsrf,
		"output":      outPath,
	}, nil
}

func writeCookieJSON(output string, cookies []browserCookie) (string, error) {
	path := expandUser(output)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("--output is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	content, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return "", err
	}
	_ = os.Chmod(path, 0o600)
	return path, nil
}

func macOSKeyFromKeychain(service string, account string) ([]byte, error) {
	args := []string{"find-generic-password", "-w", "-s", service}
	if strings.TrimSpace(account) != "" {
		args = append(args, "-a", account)
	}
	raw, err := commandOutput("security", args...)
	if err != nil {
		return nil, fmt.Errorf("could not read macOS Keychain password for service %q", service)
	}
	password := strings.TrimRight(string(raw), "\n")
	if password == "" {
		return nil, fmt.Errorf("empty Keychain password for service %q", service)
	}
	return pbkdf2SHA1([]byte(password), []byte("saltysalt"), 1003, 16), nil
}

func decryptMacOSCookieValue(host string, encrypted []byte, key []byte) (string, error) {
	if len(encrypted) == 0 {
		return "", nil
	}
	ciphertext := encrypted
	if bytes.HasPrefix(ciphertext, []byte("v10")) || bytes.HasPrefix(ciphertext, []byte("v11")) {
		ciphertext = ciphertext[3:]
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("macOS Chromium cookie encrypted_value is not a valid AES-CBC blob")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, bytes.Repeat([]byte(" "), aes.BlockSize)).CryptBlocks(plain, ciphertext)
	plain, err = unpadPKCS7(plain, aes.BlockSize)
	if err != nil {
		return "", err
	}
	hostDigest := sha256.Sum256([]byte(host))
	if bytes.HasPrefix(plain, hostDigest[:]) {
		plain = plain[len(hostDigest):]
	}
	return string(plain), nil
}

func loadWindowsMasterKey(localState string) ([]byte, error) {
	content, err := os.ReadFile(localState)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, fmt.Errorf("Windows LarkShell Local State is invalid")
	}
	osCrypt, _ := payload["os_crypt"].(map[string]any)
	encoded, _ := osCrypt["encrypted_key"].(string)
	if encoded == "" {
		return nil, fmt.Errorf("Windows LarkShell Local State does not contain os_crypt.encrypted_key")
	}
	wrapped, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(wrapped) == 0 {
		return nil, fmt.Errorf("Windows LarkShell Local State key is invalid")
	}
	if bytes.HasPrefix(wrapped, []byte("DPAPI")) {
		wrapped = wrapped[len("DPAPI"):]
	}
	key, err := dpapiUnprotectBytes(wrapped)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt Windows LarkShell Local State key: %w", err)
	}
	return key, nil
}

func decryptWindowsCookieValue(encrypted []byte, masterKey []byte) (string, error) {
	if isModernChromiumCookie(encrypted) {
		if len(masterKey) == 0 {
			return "", fmt.Errorf("Windows Chromium master key is required")
		}
		if len(encrypted) < 3+12+16 {
			return "", fmt.Errorf("Chromium cookie encrypted_value is not a valid AES-GCM blob")
		}
		block, err := aes.NewCipher(masterKey)
		if err != nil {
			return "", err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", err
		}
		plain, err := gcm.Open(nil, encrypted[3:15], encrypted[15:], nil)
		if err != nil {
			return "", fmt.Errorf("could not decrypt Windows Chromium cookie value")
		}
		return string(plain), nil
	}
	plain, err := dpapiUnprotectBytes(encrypted)
	if err != nil {
		return "", fmt.Errorf("could not decrypt Windows cookie value: %w", err)
	}
	if !utf8.Valid(plain) {
		return "", fmt.Errorf("decrypted Windows Chromium cookie value is not valid UTF-8")
	}
	return string(plain), nil
}

func isModernChromiumCookie(value []byte) bool {
	return bytes.HasPrefix(value, []byte("v10")) || bytes.HasPrefix(value, []byte("v11"))
}

func unpadPKCS7(value []byte, blockSize int) ([]byte, error) {
	if len(value) == 0 || len(value)%blockSize != 0 {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	padding := int(value[len(value)-1])
	if padding == 0 || padding > blockSize || padding > len(value) {
		return nil, fmt.Errorf("invalid PKCS7 padding")
	}
	for _, item := range value[len(value)-padding:] {
		if int(item) != padding {
			return nil, fmt.Errorf("invalid PKCS7 padding")
		}
	}
	return value[:len(value)-padding], nil
}

func pbkdf2SHA1(password []byte, salt []byte, iterations int, keyLength int) []byte {
	hashLength := sha1.Size
	blocks := (keyLength + hashLength - 1) / hashLength
	output := make([]byte, 0, blocks*hashLength)
	for blockIndex := 1; blockIndex <= blocks; blockIndex++ {
		mac := hmac.New(sha1.New, password)
		mac.Write(salt)
		var counter [4]byte
		binary.BigEndian.PutUint32(counter[:], uint32(blockIndex))
		mac.Write(counter[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha1.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		output = append(output, t...)
	}
	return output[:keyLength]
}

func chromiumExpiresToUnix(expiresUTC int64) int64 {
	value := expiresUTC/1_000_000 - 11644473600
	if value < 0 {
		return 0
	}
	return value
}

func sameSiteName(value int64) string {
	switch value {
	case -1:
		return "None"
	case 0:
		return "Lax"
	case 1:
		return "Strict"
	default:
		return ""
	}
}

func expandUser(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func valueOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
