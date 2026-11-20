package servicedesk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultRegistryURL = "http://10.103.251.43/files/ixf-toolbox/servicedesk/v1/"
	registryCacheAge   = 24 * time.Hour
	maxRegistryBytes   = 8 << 20
)

type Registry struct {
	SchemaVersion int             `json:"schemaVersion"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	Profiles      []RegistryEntry `json:"profiles"`
}

type RegistryEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type RegistryStatus struct {
	Source    string
	FetchedAt time.Time
	Stale     bool
}

func (s RegistryStatus) ApplyAllowed() bool {
	return (s.Source == "network" || s.Source == "cache") && !s.Stale
}

type RegistryLoader struct {
	HTTPClient *http.Client
	CacheDir   string
	Now        func() time.Time
}

type ProfileSummary struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Routing     Routing       `json:"routing"`
	Target      OverlayTarget `json:"target"`
}

type registryWire struct {
	SchemaVersion *int                 `json:"schemaVersion"`
	UpdatedAt     *string              `json:"updatedAt"`
	Profiles      *[]registryEntryWire `json:"profiles"`
}

type registryEntryWire struct {
	Name   *string `json:"name"`
	Path   *string `json:"path"`
	SHA256 *string `json:"sha256"`
}

type loadedRegistry struct {
	Registry Registry
	Overlays map[string]Overlay
	Bytes    map[string][]byte
}

func (l RegistryLoader) List(ctx context.Context) ([]ProfileSummary, RegistryStatus, error) {
	loaded, status, err := l.load(ctx)
	if err != nil {
		return nil, status, err
	}
	summaries := make([]ProfileSummary, 0, len(loaded.Registry.Profiles))
	for _, entry := range loaded.Registry.Profiles {
		overlay := loaded.Overlays[entry.Name]
		summaries = append(summaries, ProfileSummary{
			Name:        overlay.Name,
			Description: overlay.Description,
			Routing:     overlay.Routing,
			Target:      overlay.Target,
		})
	}
	return summaries, status, nil
}

func (l RegistryLoader) Show(ctx context.Context, name string) (Overlay, RegistryStatus, error) {
	loaded, status, err := l.load(ctx)
	if err != nil {
		return Overlay{}, status, err
	}
	for _, entry := range loaded.Registry.Profiles {
		if entry.Name == name {
			return loaded.Overlays[name], status, nil
		}
	}
	return Overlay{}, status, fmt.Errorf("ServiceDesk profile %q was not found in registry", name)
}

func (l RegistryLoader) load(ctx context.Context) (loadedRegistry, RegistryStatus, error) {
	now := l.now()
	network, err := l.loadNetwork(ctx, now)
	if err == nil {
		return network, RegistryStatus{Source: "network", FetchedAt: now}, nil
	}

	cached, cacheStatus, cacheErr := l.loadCache(now)
	if cacheErr == nil {
		return cached, cacheStatus, nil
	}
	return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("load ServiceDesk registry from network: %w; load cache: %v", err, cacheErr)
}

func (l RegistryLoader) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l RegistryLoader) loadNetwork(ctx context.Context, fetchedAt time.Time) (loadedRegistry, error) {
	indexBytes, err := l.fetch(ctx, DefaultRegistryURL+"index.json")
	if err != nil {
		return loadedRegistry{}, fmt.Errorf("fetch index: %w", err)
	}
	registry, err := decodeRegistry(indexBytes)
	if err != nil {
		return loadedRegistry{}, err
	}

	loaded := loadedRegistry{
		Registry: registry,
		Overlays: make(map[string]Overlay, len(registry.Profiles)),
		Bytes:    make(map[string][]byte, len(registry.Profiles)),
	}
	for _, entry := range registry.Profiles {
		profileURL, err := registryProfileURL(entry.Path)
		if err != nil {
			return loadedRegistry{}, err
		}
		profileBytes, err := l.fetch(ctx, profileURL)
		if err != nil {
			return loadedRegistry{}, fmt.Errorf("fetch profile %q: %w", entry.Name, err)
		}
		overlay, err := validateRegistryProfile(entry, profileBytes)
		if err != nil {
			return loadedRegistry{}, err
		}
		loaded.Overlays[entry.Name] = overlay
		loaded.Bytes[entry.Path] = profileBytes
	}
	if err := l.writeCache(loaded, indexBytes, fetchedAt); err != nil {
		return loadedRegistry{}, fmt.Errorf("cache registry: %w", err)
	}
	return loaded, nil
}

func (l RegistryLoader) loadCache(now time.Time) (loadedRegistry, RegistryStatus, error) {
	cacheDir, err := l.cacheDir()
	if err != nil {
		return loadedRegistry{}, RegistryStatus{}, err
	}
	indexPath := filepath.Join(cacheDir, "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("read cached index: %w", err)
	}
	registry, err := decodeRegistry(indexBytes)
	if err != nil {
		return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("decode cached index: %w", err)
	}
	loaded := loadedRegistry{
		Registry: registry,
		Overlays: make(map[string]Overlay, len(registry.Profiles)),
		Bytes:    make(map[string][]byte, len(registry.Profiles)),
	}
	for _, entry := range registry.Profiles {
		profilePath, err := cachedProfilePath(cacheDir, entry.Path)
		if err != nil {
			return loadedRegistry{}, RegistryStatus{}, err
		}
		profileBytes, err := os.ReadFile(profilePath)
		if err != nil {
			return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("read cached profile %q: %w", entry.Name, err)
		}
		overlay, err := validateRegistryProfile(entry, profileBytes)
		if err != nil {
			return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("validate cached profile %q: %w", entry.Name, err)
		}
		loaded.Overlays[entry.Name] = overlay
		loaded.Bytes[entry.Path] = profileBytes
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("stat cached index: %w", err)
	}
	fetchedAt := info.ModTime()
	return loaded, RegistryStatus{
		Source:    "cache",
		FetchedAt: fetchedAt,
		Stale:     !now.Before(fetchedAt.Add(registryCacheAge)),
	}, nil
}

func (l RegistryLoader) fetch(ctx context.Context, target string) ([]byte, error) {
	client := l.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	} else {
		clone := *client
		client = &clone
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	reader := io.LimitReader(response.Body, maxRegistryBytes+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(body) > maxRegistryBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxRegistryBytes)
	}
	return body, nil
}

func decodeRegistry(data []byte) (Registry, error) {
	var wire registryWire
	if err := decodeStrictJSON(data, &wire); err != nil {
		return Registry{}, fmt.Errorf("decode ServiceDesk registry index: %w", err)
	}
	if wire.SchemaVersion == nil || wire.UpdatedAt == nil || wire.Profiles == nil {
		return Registry{}, errors.New("ServiceDesk registry index is missing a required property")
	}
	if *wire.SchemaVersion != 1 {
		return Registry{}, errors.New("ServiceDesk registry schemaVersion must be 1")
	}
	updatedAt, err := time.Parse(time.RFC3339, *wire.UpdatedAt)
	if err != nil {
		return Registry{}, fmt.Errorf("ServiceDesk registry updatedAt must be RFC3339: %w", err)
	}
	registry := Registry{SchemaVersion: *wire.SchemaVersion, UpdatedAt: updatedAt, Profiles: make([]RegistryEntry, 0, len(*wire.Profiles))}
	seen := make(map[string]struct{}, len(*wire.Profiles))
	for index, entry := range *wire.Profiles {
		if entry.Name == nil || entry.Path == nil || entry.SHA256 == nil {
			return Registry{}, fmt.Errorf("ServiceDesk registry profile %d is missing a required property", index)
		}
		if !overlayNamePattern.MatchString(*entry.Name) || len(*entry.Name) > 64 {
			return Registry{}, fmt.Errorf("ServiceDesk registry profile %d has invalid name", index)
		}
		if _, exists := seen[*entry.Name]; exists {
			return Registry{}, fmt.Errorf("ServiceDesk registry contains duplicate profile name %q", *entry.Name)
		}
		seen[*entry.Name] = struct{}{}
		if err := validateRegistryPath(*entry.Path); err != nil {
			return Registry{}, fmt.Errorf("ServiceDesk registry profile %q: %w", *entry.Name, err)
		}
		if !lowerSHA256Pattern.MatchString(*entry.SHA256) {
			return Registry{}, fmt.Errorf("ServiceDesk registry profile %q has invalid sha256", *entry.Name)
		}
		registry.Profiles = append(registry.Profiles, RegistryEntry{Name: *entry.Name, Path: *entry.Path, SHA256: *entry.SHA256})
	}
	return registry, nil
}

var lowerSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validateRegistryProfile(entry RegistryEntry, data []byte) (Overlay, error) {
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != entry.SHA256 {
		return Overlay{}, fmt.Errorf("ServiceDesk profile %q sha256 does not match registry", entry.Name)
	}
	overlay, err := DecodeOverlay(data)
	if err != nil {
		return Overlay{}, fmt.Errorf("decode ServiceDesk profile %q: %w", entry.Name, err)
	}
	if err := ValidateOverlaySchema(overlay); err != nil {
		return Overlay{}, fmt.Errorf("validate ServiceDesk profile %q: %w", entry.Name, err)
	}
	if overlay.Name != entry.Name {
		return Overlay{}, fmt.Errorf("ServiceDesk profile name %q does not match registry entry %q", overlay.Name, entry.Name)
	}
	return overlay, nil
}

func validateRegistryPath(raw string) error {
	if raw == "" || strings.ContainsAny(raw, `\\?#`) || strings.HasPrefix(raw, "/") || path.Clean(raw) != raw {
		return errors.New("path must be a clean relative slash path")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != raw {
		return errors.New("path must not contain a host, query, or fragment")
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("path must not contain empty, dot, or parent segments")
		}
	}
	return nil
}

func registryProfileURL(raw string) (string, error) {
	if err := validateRegistryPath(raw); err != nil {
		return "", err
	}
	base, err := url.Parse(DefaultRegistryURL)
	if err != nil {
		return "", err
	}
	profileURL, err := base.Parse(raw)
	if err != nil || profileURL.Scheme != base.Scheme || profileURL.Host != base.Host || profileURL.RawQuery != "" || profileURL.Fragment != "" {
		return "", errors.New("profile path resolves outside the registry root")
	}
	return profileURL.String(), nil
}

func (l RegistryLoader) cacheDir() (string, error) {
	if l.CacheDir != "" {
		return l.CacheDir, nil
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve ServiceDesk cache directory: %w", err)
	}
	return filepath.Join(root, "ixf-toolbox", "servicedesk"), nil
}

func cachedProfilePath(cacheDir, raw string) (string, error) {
	if err := validateRegistryPath(raw); err != nil {
		return "", err
	}
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(raw))
	absolute, err := filepath.Abs(target)
	if err != nil || absolute != root && !strings.HasPrefix(absolute, root+string(filepath.Separator)) {
		return "", errors.New("cached profile path escapes cache directory")
	}
	return target, nil
}

func (l RegistryLoader) writeCache(loaded loadedRegistry, indexBytes []byte, fetchedAt time.Time) error {
	cacheDir, err := l.cacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return err
	}
	for profilePath, profileBytes := range loaded.Bytes {
		target, err := cachedProfilePath(cacheDir, profilePath)
		if err != nil {
			return err
		}
		if err := writeRegistryFile(target, profileBytes, 0o600); err != nil {
			return err
		}
	}
	indexPath := filepath.Join(cacheDir, "index.json")
	if err := writeRegistryFile(indexPath, indexBytes, 0o600); err != nil {
		return err
	}
	return os.Chtimes(indexPath, fetchedAt, fetchedAt)
}

func writeRegistryFile(target string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".registry-*-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, target)
}
