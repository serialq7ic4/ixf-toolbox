package servicedesk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdp "github.com/serialq7ic4/ixf-toolbox/pkg/servicedeskprofile"
)

const (
	DefaultRegistryURL = "http://10.103.251.43/files/ixf-toolbox/servicedesk/v1/"
	registryCacheAge   = 24 * time.Hour
)

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
	fetched, err := (sdp.RegistryFetcher{
		HTTPClient: l.HTTPClient,
		BaseURL:    DefaultRegistryURL,
	}).Fetch(ctx)
	if err != nil {
		return loadedRegistry{}, err
	}

	loaded := loadedRegistry{
		Registry: fetched.Registry,
		Overlays: make(map[string]Overlay, len(fetched.Profiles)),
		Bytes:    make(map[string][]byte, len(fetched.Profiles)),
	}
	for _, profile := range fetched.Profiles {
		loaded.Overlays[profile.Entry.Name] = profile.Overlay
		loaded.Bytes[profile.Entry.Path] = profile.Bytes
	}
	if err := l.writeCache(loaded, fetched.IndexBytes, fetchedAt); err != nil {
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

func decodeRegistry(data []byte) (Registry, error) {
	return sdp.DecodeRegistry(data)
}

func validateRegistryProfile(entry RegistryEntry, data []byte) (Overlay, error) {
	return sdp.ValidateRegistryProfile(entry, data)
}

func validateRegistryPath(raw string) error {
	return sdp.ValidateRegistryPath(raw)
}

func validateRegistryPaths(entries []RegistryEntry) error {
	return sdp.ValidateRegistry(Registry{
		SchemaVersion: sdp.RegistrySchemaVersion,
		UpdatedAt:     time.Unix(0, 0).UTC(),
		Profiles:      entries,
	})
}

func registryProfileURL(raw string) (string, error) {
	return sdp.ResolveRegistryURL(DefaultRegistryURL, raw)
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
