package servicedeskprofile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegistryFetcherFetchesVerifiedNoCacheRegistry(t *testing.T) {
	profile := mustReadFixture(t, "profile-4003.json")
	digest := sha256.Sum256(profile)
	hash := hex.EncodeToString(digest[:])
	profilePath := "profiles/fixture-server-hardware-repair/" + hash + ".json"
	index, err := EncodeRegistry(Registry{
		SchemaVersion: RegistrySchemaVersion,
		UpdatedAt:     time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC),
		Profiles:      []RegistryEntry{{Name: "fixture-server-hardware-repair", Path: profilePath, SHA256: hash}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cache-Control") != "no-cache" {
			t.Errorf("Cache-Control = %q", r.Header.Get("Cache-Control"))
		}
		switch r.URL.Path {
		case "/registry/index.json":
			_, _ = w.Write(index)
		case "/registry/" + profilePath:
			_, _ = w.Write(profile)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := (RegistryFetcher{BaseURL: server.URL + "/registry/", NoCache: true}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profiles) != 1 || result.Profiles[0].Overlay.Name != "fixture-server-hardware-repair" || result.IndexSHA256 != ProfileSHA256(index) {
		t.Fatalf("Fetch() = %+v", result)
	}
	if profileResult, ok := result.Profile("fixture-server-hardware-repair"); !ok || profileResult.URL != server.URL+"/registry/"+profilePath {
		t.Fatalf("Profile() = %+v, %v", profileResult, ok)
	}
}

func TestRegistryFetcherRejectsRedirectAndHashMismatch(t *testing.T) {
	profile := mustReadFixture(t, "profile-4003.json")
	for name, handler := range map[string]http.HandlerFunc{
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/other", http.StatusFound)
		},
		"hash mismatch": func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "index.json") {
				_, _ = fmt.Fprintf(w, `{"schemaVersion":1,"updatedAt":"2026-11-24T00:00:00Z","profiles":[{"name":"fixture-server-hardware-repair","path":"profiles/profile.json","sha256":"%s"}]}`, strings.Repeat("0", 64))
				return
			}
			_, _ = w.Write(profile)
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			if _, err := (RegistryFetcher{BaseURL: server.URL + "/"}).Fetch(context.Background()); err == nil {
				t.Fatal("Fetch() succeeded, want failure")
			}
		})
	}
}

func TestRegistryFetcherRejectsHTTPFailureAndOversizedResponse(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"non 2xx": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		},
		"oversized response": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxRegistryBytes+1))
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			if _, err := (RegistryFetcher{BaseURL: server.URL + "/"}).Fetch(context.Background()); err == nil {
				t.Fatal("Fetch() succeeded, want HTTP boundary failure")
			}
		})
	}
}

func TestRegistryFetcherDefaultClientIgnoresProxyEnvironment(t *testing.T) {
	proxyUsed := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyUsed = true
		_, _ = w.Write([]byte(`{"schemaVersion":1,"updatedAt":"2026-11-24T00:00:00Z","profiles":[]}`))
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	if _, err := (RegistryFetcher{BaseURL: "http://registry.invalid/"}).Fetch(context.Background()); err == nil {
		t.Fatal("Fetch() succeeded through environment proxy")
	}
	if proxyUsed {
		t.Fatal("Fetch() inherited HTTP_PROXY")
	}
}

func TestResolveRegistryURLValidatesBaseAndPath(t *testing.T) {
	got, err := ResolveRegistryURL("https://registry.example/files/v1/", "profiles/hardware/profile.json")
	if err != nil || got != "https://registry.example/files/v1/profiles/hardware/profile.json" {
		t.Fatalf("ResolveRegistryURL() = %q, %v", got, err)
	}

	for _, test := range []struct {
		base string
		path string
	}{
		{base: "registry.example/files/v1/", path: "profile.json"},
		{base: "ftp://registry.example/files/v1/", path: "profile.json"},
		{base: "https://user@registry.example/files/v1/", path: "profile.json"},
		{base: "https://registry.example/files/v1/?query=1", path: "profile.json"},
		{base: "https://registry.example/files/v1/#fragment", path: "profile.json"},
		{base: "https://registry.example/files/v1", path: "profile.json"},
		{base: "https://registry.example/files/v1/", path: "../profile.json"},
		{base: "https://registry.example/files/v1/", path: "//other.example/profile.json"},
	} {
		if _, err := ResolveRegistryURL(test.base, test.path); err == nil {
			t.Fatalf("ResolveRegistryURL(%q, %q) succeeded", test.base, test.path)
		}
	}
}
