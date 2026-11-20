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
	"sort"
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

	writeCacheFile   func(string, []byte, os.FileMode) error
	renameCachePath  func(string, string) error
	chtimesCachePath func(string, time.Time, time.Time) error
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
	if err := validateCachePath(cacheDir, indexPath, cacheRegularFile, false); err != nil {
		return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("validate ServiceDesk cache index: %w", err)
	}
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("read cached index: %w", err)
	}
	registry, err := decodeRegistry(indexBytes)
	if err != nil {
		return loadedRegistry{}, RegistryStatus{}, fmt.Errorf("decode cached index: %w", err)
	}
	if err := validateCacheLayout(cacheDir, registry.Profiles, false); err != nil {
		return loadedRegistry{}, RegistryStatus{}, err
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
	info, err := os.Lstat(indexPath)
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
		client = newDirectHTTPClient(defaultTimeout)
	} else {
		clone := *client
		client = &clone
		if client.Transport == nil {
			client.Transport = newDirectHTTPClient(defaultTimeout).Transport
		}
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
	if err := validateRegistryPaths(registry.Profiles); err != nil {
		return Registry{}, err
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

func validateRegistryPaths(entries []RegistryEntry) error {
	paths := make([]string, 0, len(entries)+1)
	paths = append(paths, "index.json")
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	sort.Strings(paths)
	for index := 1; index < len(paths); index++ {
		previous, current := paths[index-1], paths[index]
		if current == previous || strings.HasPrefix(current, previous+"/") {
			return fmt.Errorf("ServiceDesk registry paths %q and %q cannot share a file path prefix", previous, current)
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
	var cacheDir string
	if l.CacheDir != "" {
		cacheDir = l.CacheDir
	} else {
		root, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("resolve ServiceDesk cache directory: %w", err)
		}
		cacheDir = filepath.Join(root, "ixf-toolbox", "servicedesk")
	}
	absolute, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", fmt.Errorf("resolve ServiceDesk cache directory: %w", err)
	}
	return absolute, nil
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
	if err := validateRegistryPaths(loaded.Registry.Profiles); err != nil {
		return err
	}
	if err := validateCachePath(cacheDir, cacheDir, cacheDirectory, true); err != nil {
		return fmt.Errorf("validate ServiceDesk cache directory: %w", err)
	}
	if err := validateCacheLayout(cacheDir, loaded.Registry.Profiles, true); err != nil {
		return err
	}
	parent := filepath.Dir(cacheDir)
	if err := ensureCacheParent(cacheDir); err != nil {
		return fmt.Errorf("validate ServiceDesk cache parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(cacheDir)+".staging-")
	if err != nil {
		return err
	}
	stageActive := true
	defer func() {
		if stageActive {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := validateCachePath(stage, stage, cacheDirectory, false); err != nil {
		return err
	}
	writeCacheFile := l.writeCacheFile
	if writeCacheFile == nil {
		writeCacheFile = writeRegistryFile
	}
	for _, entry := range loaded.Registry.Profiles {
		profileBytes := loaded.Bytes[entry.Path]
		target, err := cachedProfilePath(stage, entry.Path)
		if err != nil {
			return err
		}
		if err := writeCacheFile(target, profileBytes, 0o600); err != nil {
			return err
		}
	}
	indexPath := filepath.Join(stage, "index.json")
	if err := writeCacheFile(indexPath, indexBytes, 0o600); err != nil {
		return err
	}
	chtimesCachePath := l.chtimesCachePath
	if chtimesCachePath == nil {
		chtimesCachePath = os.Chtimes
	}
	if err := chtimesCachePath(indexPath, fetchedAt, fetchedAt); err != nil {
		return err
	}

	renameCachePath := l.renameCachePath
	if renameCachePath == nil {
		renameCachePath = os.Rename
	}
	cacheInfo, err := os.Lstat(cacheDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hasOldCache := err == nil
	if hasOldCache && (cacheInfo.Mode()&os.ModeSymlink != 0 || !cacheInfo.IsDir()) {
		return errors.New("ServiceDesk cache directory is not a regular directory")
	}
	backup := ""
	if hasOldCache {
		backup, err = os.MkdirTemp(parent, "."+filepath.Base(cacheDir)+".backup-")
		if err != nil {
			return err
		}
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
		if err := renameCachePath(cacheDir, backup); err != nil {
			return fmt.Errorf("stage old ServiceDesk cache: %w", err)
		}
	}
	if err := renameCachePath(stage, cacheDir); err != nil {
		if hasOldCache {
			if restoreErr := renameCachePath(backup, cacheDir); restoreErr != nil {
				return fmt.Errorf("activate ServiceDesk cache: %w; restore old cache: %v", err, restoreErr)
			}
		}
		return fmt.Errorf("activate ServiceDesk cache: %w", err)
	}
	stageActive = false
	if hasOldCache {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove old ServiceDesk cache backup: %w", err)
		}
	}
	return nil
}

type cachePathKind uint8

const (
	cacheDirectory cachePathKind = iota
	cacheRegularFile
)

func validateCacheLayout(cacheDir string, entries []RegistryEntry, allowMissing bool) error {
	if err := validateCachePath(cacheDir, cacheDir, cacheDirectory, allowMissing); err != nil {
		return fmt.Errorf("validate ServiceDesk cache root: %w", err)
	}
	indexPath := filepath.Join(cacheDir, "index.json")
	if err := validateCachePath(cacheDir, indexPath, cacheRegularFile, allowMissing); err != nil {
		return fmt.Errorf("validate ServiceDesk cache index: %w", err)
	}
	for _, entry := range entries {
		profilePath, err := cachedProfilePath(cacheDir, entry.Path)
		if err != nil {
			return err
		}
		if err := validateCachePath(cacheDir, profilePath, cacheRegularFile, allowMissing); err != nil {
			return fmt.Errorf("validate cached profile %q: %w", entry.Name, err)
		}
	}
	return nil
}

func ensureCacheParent(cacheDir string) error {
	parent := filepath.Dir(cacheDir)
	if err := validatePathComponents(parent, cacheDirectory, true); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	return validatePathComponents(parent, cacheDirectory, false)
}

func validateCachePath(cacheRoot, target string, kind cachePathKind, allowMissing bool) error {
	absoluteRoot, err := filepath.Abs(cacheRoot)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absoluteRoot, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes cache root %q", target, cacheRoot)
	}
	return validatePathComponents(absolute, kind, allowMissing)
}

func validatePathComponents(target string, kind cachePathKind, allowMissing bool) error {
	absolute, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	filesystemRoot := filepath.VolumeName(absolute) + string(filepath.Separator)
	if absolute == filesystemRoot {
		return validateExistingPathComponent(absolute, kind)
	}
	relative := strings.TrimPrefix(absolute, filesystemRoot)
	missing := false
	current := filesystemRoot
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			missing = true
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %q is a symlink", current)
		}
		if current == absolute {
			if kind == cacheDirectory && !info.IsDir() {
				return fmt.Errorf("path %q is not a directory", current)
			}
			if kind == cacheRegularFile && !info.Mode().IsRegular() {
				return fmt.Errorf("path %q is not a regular file", current)
			}
		} else if !info.IsDir() {
			return fmt.Errorf("path component %q is not a directory", current)
		}
	}
	if missing && !allowMissing {
		return os.ErrNotExist
	}
	return nil
}

func validateExistingPathComponent(path string, kind cachePathKind) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path component %q is a symlink", path)
	}
	if kind == cacheDirectory && !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", path)
	}
	if kind == cacheRegularFile && !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}
	return nil
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
