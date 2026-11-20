package servicedesk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegistryLoadsAndVerifiesNetworkIndex(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	profile := mustReadFixture(t, "profile-4003.json")
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	index := registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile)
	server := registryServer(t, index, map[string][]byte{profilePath: profile})
	defer server.Close()

	cacheDir := t.TempDir()
	loader := registryLoaderFor(t, server.URL, cacheDir, now)

	summaries, status, err := loader.List(testContext(t))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].Name != "fixture-server-hardware-repair" {
		t.Fatalf("List() = %+v, want one fixture summary", summaries)
	}
	if status.Source != "network" || status.Stale || !status.FetchedAt.Equal(now) {
		t.Fatalf("network status = %+v, want network, fresh, fetchedAt %s", status, now)
	}

	got, showStatus, err := loader.Show(testContext(t), "fixture-server-hardware-repair")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if got.Name != "fixture-server-hardware-repair" || showStatus != status {
		t.Fatalf("Show() = %q, %+v; want fixture and %+v", got.Name, showStatus, status)
	}
	if cached, err := os.ReadFile(filepath.Join(cacheDir, filepath.FromSlash(profilePath))); err != nil {
		t.Fatalf("cached profile read error = %v", err)
	} else if !bytes.Equal(cached, profile) {
		t.Fatalf("cached profile bytes changed during cache write")
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); err != nil {
		t.Fatalf("cached index was not written: %v", err)
	}
}

func TestRegistryListReturnsRoutingSummaryWithoutDefaultsOrBaseline(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	profile := mustReadFixture(t, "profile-4003.json")
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	server := registryServer(t, registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile), map[string][]byte{profilePath: profile})
	defer server.Close()

	loader := registryLoaderFor(t, server.URL, t.TempDir(), now)
	summaries, _, err := loader.List(testContext(t))
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := summaries[0]; got.Description != "Fixture profile for server hardware repair" || got.Routing.DisplayName != "Server hardware repair" || got.Target != (OverlayTarget{BusinessID: 15, ServiceID: 4003}) {
		t.Fatalf("summary = %+v, want public routing summary", got)
	}
	encoded, err := json.Marshal(summaries[0])
	if err != nil {
		t.Fatalf("json.Marshal(summary) error = %v", err)
	}
	for _, forbidden := range []string{"baseline", "fields", "default", "metadataHash", "snapshotId"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("summary JSON contains forbidden %q: %s", forbidden, encoded)
		}
	}
}

func TestRegistryRejectsUnknownSchemaDuplicateNameAndPathTraversal(t *testing.T) {
	profile := mustReadFixture(t, "profile-4003.json")
	const goodPath = "profiles/fixture-server-hardware-repair.json"
	paths := []string{"../profile.json", "profiles/../profile.json", "/profile.json", "profiles\\profile.json", "profiles/profile.json?x=1", "profiles/profile.json#part", "."}
	tests := []struct {
		name  string
		index []byte
	}{
		{name: "unknown index property", index: []byte(`{"schemaVersion":1,"updatedAt":"2026-11-20T12:00:00Z","profiles":[],"extra":true}`)},
		{name: "duplicate profile name", index: registryDuplicateIndexJSON(t, profile, goodPath)},
		{name: "invalid updatedAt", index: []byte(`{"schemaVersion":1,"updatedAt":"not-a-time","profiles":[],}`)},
	}
	for _, profilePath := range paths {
		tests = append(tests, struct {
			name  string
			index []byte
		}{name: "invalid path " + profilePath, index: registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile)})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := registryServer(t, test.index, map[string][]byte{goodPath: profile})
			defer server.Close()
			loader := registryLoaderFor(t, server.URL, t.TempDir(), time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
			if _, _, err := loader.List(testContext(t)); err == nil {
				t.Fatal("List() error = nil, want malformed network index rejection")
			}
		})
	}
}

func TestRegistryRejectsHashOrOverlayNameMismatch(t *testing.T) {
	valid := mustReadFixture(t, "profile-4003.json")
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	tests := []struct {
		name    string
		profile []byte
		sha256  string
	}{
		{name: "hash mismatch", profile: append([]byte{}, valid...), sha256: strings.Repeat("0", 64)},
		{name: "uppercase hash", profile: append([]byte{}, valid...), sha256: strings.ToUpper(digest(valid))},
		{name: "overlay name mismatch", profile: bytes.Replace(valid, []byte(`"name": "fixture-server-hardware-repair"`), []byte(`"name": "other-profile"`), 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			indexHash := test.sha256
			if indexHash == "" {
				indexHash = digest(test.profile)
			}
			index := registryIndexJSONWithHash(t, "fixture-server-hardware-repair", profilePath, indexHash)
			server := registryServer(t, index, map[string][]byte{profilePath: test.profile})
			defer server.Close()
			cacheDir := t.TempDir()
			loader := registryLoaderFor(t, server.URL, cacheDir, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
			if _, _, err := loader.List(testContext(t)); err == nil {
				t.Fatal("List() error = nil, want integrity/name rejection")
			}
			if _, err := os.Stat(filepath.Join(cacheDir, "index.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed network response left cache index, stat error = %v", err)
			}
		})
	}

	t.Run("failed network validation preserves valid cache", func(t *testing.T) {
		cacheDir := t.TempDir()
		validServer := registryServer(t, registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, valid), map[string][]byte{profilePath: valid})
		initialLoader := registryLoaderFor(t, validServer.URL, cacheDir, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
		if _, _, err := initialLoader.List(testContext(t)); err != nil {
			t.Fatalf("populate cache List() error = %v", err)
		}
		validServer.Close()

		invalidServer := registryServer(t, registryIndexJSONWithHash(t, "fixture-server-hardware-repair", profilePath, strings.Repeat("0", 64)), map[string][]byte{profilePath: valid})
		defer invalidServer.Close()
		loader := registryLoaderFor(t, invalidServer.URL, cacheDir, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
		_, status, err := loader.List(testContext(t))
		if err != nil || status.Source != "cache" {
			t.Fatalf("fallback List() = status %+v, error %v; want preserved cache", status, err)
		}
		cached, err := os.ReadFile(filepath.Join(cacheDir, filepath.FromSlash(profilePath)))
		if err != nil {
			t.Fatalf("read preserved profile: %v", err)
		}
		if !bytes.Equal(cached, valid) {
			t.Fatal("failed network validation overwrote valid cached profile")
		}
	})
}

func TestRegistryFallsBackToFreshCache(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	profile := mustReadFixture(t, "profile-4003.json")
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	cacheDir := t.TempDir()
	server := registryServer(t, registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile), map[string][]byte{profilePath: profile})
	loader := registryLoaderFor(t, server.URL, cacheDir, now)
	if _, _, err := loader.List(testContext(t)); err != nil {
		t.Fatalf("initial List() error = %v", err)
	}
	server.Close()

	loader = registryLoaderFor(t, "http://127.0.0.1:1", cacheDir, now.Add(time.Hour))
	_, status, err := loader.List(testContext(t))
	if err != nil {
		t.Fatalf("cached List() error = %v", err)
	}
	if status.Source != "cache" || status.Stale || !status.ApplyAllowed() || !status.FetchedAt.Equal(now) {
		t.Fatalf("fresh cache status = %+v, want cache, fresh, apply allowed, fetchedAt %s", status, now)
	}
}

func TestRegistryStaleCacheReadAndDryRunAllowedApplyBlocked(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	profile := mustReadFixture(t, "profile-4003.json")
	const profilePath = "profiles/fixture-server-hardware-repair.json"
	cacheDir := t.TempDir()
	server := registryServer(t, registryIndexJSON(t, "fixture-server-hardware-repair", profilePath, profile), map[string][]byte{profilePath: profile})
	loader := registryLoaderFor(t, server.URL, cacheDir, now)
	if _, _, err := loader.List(testContext(t)); err != nil {
		t.Fatalf("initial List() error = %v", err)
	}
	server.Close()
	indexPath := filepath.Join(cacheDir, "index.json")
	if err := os.Chtimes(indexPath, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatalf("age cache: %v", err)
	}

	loader = registryLoaderFor(t, "http://127.0.0.1:1", cacheDir, now)
	_, status, err := loader.List(testContext(t))
	if err != nil {
		t.Fatalf("stale cached List() error = %v", err)
	}
	if status.Source != "cache" || !status.Stale || status.ApplyAllowed() {
		t.Fatalf("stale cache status = %+v, want cache, stale, apply blocked", status)
	}
	if _, showStatus, err := loader.Show(testContext(t), "fixture-server-hardware-repair"); err != nil || showStatus != status {
		t.Fatalf("stale cached Show() = status %+v, error %v; want readable stale cache", showStatus, err)
	}
}

func TestRegistryCorruptCacheIsNotUsed(t *testing.T) {
	cacheDir := t.TempDir()
	profilePath := filepath.Join(cacheDir, "profiles", "fixture-server-hardware-repair.json")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		t.Fatalf("create cache profile directory: %v", err)
	}
	profile := mustReadFixture(t, "profile-4003.json")
	if err := os.WriteFile(filepath.Join(cacheDir, "index.json"), registryIndexJSON(t, "fixture-server-hardware-repair", "profiles/fixture-server-hardware-repair.json", profile), 0o600); err != nil {
		t.Fatalf("write cache index: %v", err)
	}
	if err := os.WriteFile(profilePath, []byte("corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt cache profile: %v", err)
	}

	loader := registryLoaderFor(t, "http://127.0.0.1:1", cacheDir, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))
	if _, _, err := loader.List(testContext(t)); err == nil {
		t.Fatal("List() error = nil, want corrupt cache rejection")
	}
}

func registryLoaderFor(t *testing.T, serverURL, cacheDir string, now time.Time) RegistryLoader {
	t.Helper()
	base, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("url.Parse(serverURL) error = %v", err)
	}
	return RegistryLoader{
		HTTPClient: &http.Client{Transport: rewriteRegistryTransport{base: base}},
		CacheDir:   cacheDir,
		Now:        func() time.Time { return now },
	}
}

type rewriteRegistryTransport struct {
	base *url.URL
}

func (t rewriteRegistryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = t.base.Scheme
	clone.URL.Host = t.base.Host
	clone.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func registryServer(t *testing.T, index []byte, profiles map[string][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if path.Base(request.URL.Path) == "index.json" {
			_, _ = w.Write(index)
			return
		}
		for profilePath, body := range profiles {
			if path.Base(request.URL.Path) == path.Base(profilePath) {
				_, _ = w.Write(body)
				return
			}
		}
		http.NotFound(w, request)
	}))
}

func registryIndexJSON(t *testing.T, name, profilePath string, profile []byte) []byte {
	t.Helper()
	return registryIndexJSONWithHash(t, name, profilePath, digest(profile))
}

func registryIndexJSONWithHash(t *testing.T, name, profilePath, hash string) []byte {
	t.Helper()
	value := fmt.Sprintf(`{"schemaVersion":1,"updatedAt":"2026-11-20T12:00:00Z","profiles":[{"name":%q,"path":%q,"sha256":%q}]}`, name, profilePath, hash)
	return []byte(value)
}

func registryDuplicateIndexJSON(t *testing.T, profile []byte, profilePath string) []byte {
	t.Helper()
	hash := digest(profile)
	return []byte(fmt.Sprintf(`{"schemaVersion":1,"updatedAt":"2026-11-20T12:00:00Z","profiles":[{"name":"duplicate","path":%q,"sha256":%q},{"name":"duplicate","path":%q,"sha256":%q}]}`, profilePath, hash, profilePath, hash))
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
