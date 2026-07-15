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
	dbPath, err := resolveMacOSCookieDB(config)
	if err != nil {
		return nil, err
	}
	hostLike := valueOrDefault(config.HostLike, DefaultHostLike)
	rows, err := readMacOSRows(dbPath, hostLike)
	if err != nil {
		return nil, err
	}
	needsDecrypt := false
	for _, row := range rows {
		if row.Value == "" && len(row.EncryptedValue) > 0 {
			needsDecrypt = true
			break
		}
	}
	var key []byte
	if needsDecrypt {
		service := valueOrDefault(config.KeychainService, DefaultKeychainService)
		key, err = macOSKeyFromKeychain(service, config.KeychainAccount)
		if err != nil {
			return nil, err
		}
	}
	cookies := make([]browserCookie, 0, len(rows))
	for _, row := range rows {
		value := row.Value
		if value == "" && len(row.EncryptedValue) > 0 {
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
	return writeExport("macos-larkshell", config.Output, cookies)
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
	if strings.TrimSpace(config.CookiesDB) != "" {
		return expandUser(config.CookiesDB), nil
	}
	appSupport := expandUser(valueOrDefault(config.AppSupport, DefaultAppSupport))
	pattern := filepath.Join(appSupport, "aha", "users", "*", "profile_explorer", "Cookies")
	candidates, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, leftErr := os.Stat(candidates[i])
		right, rightErr := os.Stat(candidates[j])
		if leftErr != nil || rightErr != nil {
			return candidates[i] < candidates[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no profile_explorer/Cookies found under %s", appSupport)
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
	return dpapiUnprotect(wrapped)
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
	plain, err := dpapiUnprotect(encrypted)
	if err != nil {
		return "", err
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
