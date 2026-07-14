package update

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSelfUpdateApplyDownloadsVerifiesAndReplacesTarget(t *testing.T) {
	version := "1.3.0"
	artifactName := ArtifactName(version, runtime.GOOS, runtime.GOARCH)
	replacement := []byte("new-go-binary\n")
	checksum := fmt.Sprintf("%x", sha256.Sum256(replacement))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + artifactName:
			_, _ = w.Write(replacement)
		case "/ixf_1.3.0_checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", checksum, artifactName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "ixf-target")
	if err := os.WriteFile(target, []byte("old-go-binary\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	payload, err := SelfUpdateWithOptions(SelfUpdateOptions{
		Repo:           "serialq7ic4/ixf-toolbox",
		CurrentVersion: "1.2.0",
		Release: Release{
			TagName: "v1.3.0",
			HTMLURL: "https://github.example/releases/v1.3.0",
			Assets: []Asset{
				{Name: artifactName, BrowserDownloadURL: server.URL + "/" + artifactName},
				{Name: "ixf_1.3.0_checksums.txt", BrowserDownloadURL: server.URL + "/ixf_1.3.0_checksums.txt"},
			},
		},
		Apply:      true,
		TargetPath: target,
	})
	if err != nil {
		t.Fatalf("SelfUpdateWithOptions returned error: %v", err)
	}

	if payload["applied"] != true {
		t.Fatalf("applied = %#v, want true", payload["applied"])
	}
	if payload["checksumVerified"] != true {
		t.Fatalf("checksumVerified = %#v, want true", payload["checksumVerified"])
	}
	updated, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(updated) != string(replacement) {
		t.Fatalf("target content = %q, want %q", updated, replacement)
	}
}

func TestSelfUpdateApplyRejectsChecksumMismatchWithoutReplacingTarget(t *testing.T) {
	version := "1.3.0"
	artifactName := ArtifactName(version, runtime.GOOS, runtime.GOARCH)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + artifactName:
			_, _ = w.Write([]byte("tampered-binary\n"))
		case "/ixf_1.3.0_checksums.txt":
			_, _ = fmt.Fprintf(w, "%064x  %s\n", 1, artifactName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "ixf-target")
	original := []byte("old-go-binary\n")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := SelfUpdateWithOptions(SelfUpdateOptions{
		Repo:           "serialq7ic4/ixf-toolbox",
		CurrentVersion: "1.2.0",
		Release: Release{
			TagName: "v1.3.0",
			HTMLURL: "https://github.example/releases/v1.3.0",
			Assets: []Asset{
				{Name: artifactName, BrowserDownloadURL: server.URL + "/" + artifactName},
				{Name: "ixf_1.3.0_checksums.txt", BrowserDownloadURL: server.URL + "/ixf_1.3.0_checksums.txt"},
			},
		},
		Apply:      true,
		TargetPath: target,
	})
	if err == nil {
		t.Fatal("SelfUpdateWithOptions accepted a checksum mismatch")
	}

	unchanged, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(original) {
		t.Fatalf("target content = %q, want unchanged %q", unchanged, original)
	}
}
