package servicedesk

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCredentialPrecedence(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	explicitPath := filepath.Join(home, "explicit-key")
	defaultPath := filepath.Join(home, ".config", "ixf-toolbox", "servicedesk-api-key")

	tests := []struct {
		name         string
		explicitFile string
		lookup       func(string) (string, bool)
		files        map[string]string
		wantKey      string
		wantSource   string
		wantReads    []string
	}{
		{
			name:         "environment wins without touching files",
			explicitFile: explicitPath,
			lookup:       func(string) (string, bool) { return "  env-secret  ", true },
			files:        map[string]string{explicitPath: "file-secret"},
			wantKey:      "env-secret",
			wantSource:   "environment",
		},
		{
			name:         "explicit file wins over default file",
			explicitFile: explicitPath,
			lookup:       func(string) (string, bool) { return "", false },
			files: map[string]string{
				explicitPath: "  explicit-secret\n",
				defaultPath:  "default-secret",
			},
			wantKey:    "explicit-secret",
			wantSource: explicitPath,
			wantReads:  []string{explicitPath},
		},
		{
			name:   "default file is the final source",
			lookup: func(string) (string, bool) { return "", false },
			files: map[string]string{
				defaultPath: "default-secret\n",
			},
			wantKey:    "default-secret",
			wantSource: defaultPath,
			wantReads:  []string{defaultPath},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var reads []string
			loader := CredentialLoader{
				LookupEnv: test.lookup,
				ReadFile: func(path string) ([]byte, error) {
					reads = append(reads, path)
					value, ok := test.files[path]
					if !ok {
						return nil, os.ErrNotExist
					}
					return []byte(value), nil
				},
				Stat: func(path string) (fs.FileInfo, error) {
					if _, ok := test.files[path]; !ok {
						return nil, os.ErrNotExist
					}
					return stubFileInfo{mode: 0o600}, nil
				},
				GOOS:    "linux",
				HomeDir: home,
			}

			got, err := loader.Load(test.explicitFile)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.APIKey != test.wantKey || got.Source != test.wantSource {
				t.Fatalf("Load() = %+v, want key %q from %q", got, test.wantKey, test.wantSource)
			}
			if strings.Join(reads, "|") != strings.Join(test.wantReads, "|") {
				t.Fatalf("file reads = %v, want %v", reads, test.wantReads)
			}
		})
	}
}

func TestLoadCredentialRejectsEmptyAndUnsafeFiles(t *testing.T) {
	t.Parallel()

	const secret = "credential-value-must-not-appear"
	tests := []struct {
		name    string
		goos    string
		mode    fs.FileMode
		content string
		isFile  bool
	}{
		{name: "empty after trimming", goos: "linux", mode: 0o600, content: " \n\t", isFile: true},
		{name: "group readable on Unix", goos: "linux", mode: 0o640, content: secret, isFile: true},
		{name: "world readable on Unix", goos: "darwin", mode: 0o604, content: secret, isFile: true},
		{name: "not a regular file", goos: "linux", mode: fs.ModeDir | 0o700, content: secret, isFile: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			loader := CredentialLoader{
				LookupEnv: func(string) (string, bool) { return "", false },
				ReadFile:  func(string) ([]byte, error) { return []byte(test.content), nil },
				Stat: func(string) (fs.FileInfo, error) {
					return stubFileInfo{mode: test.mode, regular: test.isFile}, nil
				},
				GOOS:    test.goos,
				HomeDir: t.TempDir(),
			}

			_, err := loader.Load("credential-file")
			if err == nil {
				t.Fatal("Load() error = nil, want rejection")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("Load() error disclosed credential content")
			}
		})
	}

	t.Run("empty environment is rejected without file fallback", func(t *testing.T) {
		loader := CredentialLoader{
			LookupEnv: func(string) (string, bool) { return " \n", true },
			ReadFile:  func(string) ([]byte, error) { return []byte("fallback-secret"), nil },
			Stat:      func(string) (fs.FileInfo, error) { return stubFileInfo{mode: 0o600}, nil },
			GOOS:      "linux",
			HomeDir:   t.TempDir(),
		}
		if _, err := loader.Load(""); err == nil {
			t.Fatal("Load() error = nil, want empty environment rejection")
		}
	})

	t.Run("Windows accepts regular readable file without Unix mode check", func(t *testing.T) {
		loader := CredentialLoader{
			LookupEnv: func(string) (string, bool) { return "", false },
			ReadFile:  func(string) ([]byte, error) { return []byte("windows-secret"), nil },
			Stat:      func(string) (fs.FileInfo, error) { return stubFileInfo{mode: 0o644}, nil },
			GOOS:      "windows",
			HomeDir:   t.TempDir(),
		}
		if _, err := loader.Load("credential-file"); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
	})
}

func TestLoadBusinessIDPrecedence(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	defaultPath := filepath.Join(home, ".config", "ixf-toolbox", "servicedesk-business-id")
	tests := []struct {
		name       string
		explicit   int
		envValue   string
		envSet     bool
		fileValue  string
		fileErr    error
		wantID     int
		wantSource string
		configured bool
		wantErr    bool
		wantReads  int
	}{
		{name: "explicit wins", explicit: 15, envValue: "99", envSet: true, fileValue: "100", wantID: 15, wantSource: "flag", configured: true},
		{name: "environment wins", envValue: "23", envSet: true, fileValue: "100", wantID: 23, wantSource: "environment", configured: true},
		{name: "default file", fileValue: " 42\n", wantID: 42, wantSource: defaultPath, configured: true, wantReads: 1},
		{name: "missing default is unconfigured", fileErr: os.ErrNotExist, wantReads: 1},
		{name: "negative explicit", explicit: -1, wantErr: true},
		{name: "zero environment", envValue: "0", envSet: true, wantErr: true},
		{name: "malformed environment", envValue: "15x", envSet: true, wantErr: true},
		{name: "empty environment", envValue: " ", envSet: true, wantErr: true},
		{name: "negative file", fileValue: "-2", wantErr: true, wantReads: 1},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reads := 0
			loader := BusinessIDLoader{
				LookupEnv: func(string) (string, bool) { return test.envValue, test.envSet },
				ReadFile: func(path string) ([]byte, error) {
					reads++
					if path != defaultPath {
						t.Fatalf("ReadFile() path = %q, want %q", path, defaultPath)
					}
					return []byte(test.fileValue), test.fileErr
				},
				HomeDir: home,
			}

			id, source, configured, err := loader.Load(test.explicit)
			if (err != nil) != test.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && (id != test.wantID || source != test.wantSource || configured != test.configured) {
				t.Fatalf("Load() = (%d, %q, %v), want (%d, %q, %v)", id, source, configured, test.wantID, test.wantSource, test.configured)
			}
			if reads != test.wantReads {
				t.Fatalf("ReadFile() calls = %d, want %d", reads, test.wantReads)
			}
		})
	}
}

type stubFileInfo struct {
	mode    fs.FileMode
	regular bool
}

func (s stubFileInfo) Name() string       { return "credential-file" }
func (s stubFileInfo) Size() int64        { return 0 }
func (s stubFileInfo) Mode() fs.FileMode  { return s.mode }
func (s stubFileInfo) ModTime() time.Time { return time.Time{} }
func (s stubFileInfo) IsDir() bool        { return s.mode.IsDir() }
func (s stubFileInfo) Sys() any           { return nil }

func (s stubFileInfo) Type() fs.FileMode {
	return s.mode.Type()
}

func (s stubFileInfo) Info() (fs.FileInfo, error) {
	return s, nil
}

func (s stubFileInfo) isRegular() bool {
	if s.regular {
		return true
	}
	return s.mode.IsRegular()
}

var _ fs.FileInfo = stubFileInfo{}
var _ fs.DirEntry = stubFileInfo{}
var _ = errors.Is
