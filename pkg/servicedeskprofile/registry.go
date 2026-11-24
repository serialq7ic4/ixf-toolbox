package servicedeskprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

const RegistrySchemaVersion = 1

const maxRegistryBytes = 8 << 20

var lowerSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

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

func DecodeRegistry(data []byte) (Registry, error) {
	var wire registryWire
	if err := decodeStrictJSON(data, &wire); err != nil {
		return Registry{}, fmt.Errorf("decode ServiceDesk registry index: %w", err)
	}
	if wire.SchemaVersion == nil || wire.UpdatedAt == nil || wire.Profiles == nil {
		return Registry{}, errors.New("ServiceDesk registry index is missing a required property")
	}
	updatedAt, err := time.Parse(time.RFC3339, *wire.UpdatedAt)
	if err != nil {
		return Registry{}, fmt.Errorf("ServiceDesk registry updatedAt must be RFC3339: %w", err)
	}
	registry := Registry{SchemaVersion: *wire.SchemaVersion, UpdatedAt: updatedAt, Profiles: make([]RegistryEntry, 0, len(*wire.Profiles))}
	for index, entry := range *wire.Profiles {
		if entry.Name == nil || entry.Path == nil || entry.SHA256 == nil {
			return Registry{}, fmt.Errorf("ServiceDesk registry profile %d is missing a required property", index)
		}
		registry.Profiles = append(registry.Profiles, RegistryEntry{Name: *entry.Name, Path: *entry.Path, SHA256: *entry.SHA256})
	}
	if err := ValidateRegistry(registry); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func ValidateRegistry(registry Registry) error {
	if registry.SchemaVersion != RegistrySchemaVersion {
		return fmt.Errorf("ServiceDesk registry schemaVersion must be %d", RegistrySchemaVersion)
	}
	if registry.UpdatedAt.IsZero() {
		return errors.New("ServiceDesk registry updatedAt is required")
	}
	seen := make(map[string]struct{}, len(registry.Profiles))
	paths := map[string]struct{}{"index.json": {}}
	for index, entry := range registry.Profiles {
		if !IsValidProfileName(entry.Name) {
			return fmt.Errorf("ServiceDesk registry profile %d has invalid name", index)
		}
		if _, exists := seen[entry.Name]; exists {
			return fmt.Errorf("ServiceDesk registry contains duplicate profile name %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
		if err := ValidateRegistryPath(entry.Path); err != nil {
			return fmt.Errorf("ServiceDesk registry profile %q: %w", entry.Name, err)
		}
		if !lowerSHA256Pattern.MatchString(entry.SHA256) {
			return fmt.Errorf("ServiceDesk registry profile %q has invalid sha256", entry.Name)
		}
		if _, exists := paths[entry.Path]; exists {
			return fmt.Errorf("ServiceDesk registry path %q is already in use", entry.Path)
		}
		paths[entry.Path] = struct{}{}
	}
	for _, entry := range registry.Profiles {
		for parent := path.Dir(entry.Path); parent != "."; parent = path.Dir(parent) {
			if _, exists := paths[parent]; exists {
				return fmt.Errorf("ServiceDesk registry paths %q and %q cannot share a file path prefix", parent, entry.Path)
			}
		}
	}
	return nil
}

func ValidateRegistryPath(raw string) error {
	if raw == "" || strings.ContainsAny(raw, `\?#`) || strings.HasPrefix(raw, "/") || path.Clean(raw) != raw {
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

func ValidateRegistryProfile(entry RegistryEntry, data []byte) (Overlay, error) {
	if ProfileSHA256(data) != entry.SHA256 {
		return Overlay{}, fmt.Errorf("ServiceDesk profile %q sha256 does not match registry", entry.Name)
	}
	overlay, err := DecodeOverlay(data)
	if err != nil {
		return Overlay{}, fmt.Errorf("decode ServiceDesk profile %q: %w", entry.Name, err)
	}
	if overlay.Name != entry.Name {
		return Overlay{}, fmt.Errorf("ServiceDesk profile name %q does not match registry entry %q", overlay.Name, entry.Name)
	}
	return overlay, nil
}

func ProfileSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func ContentAddressedProfilePath(name, sha256Hex string) (string, error) {
	if !IsValidProfileName(name) {
		return "", errors.New("ServiceDesk profile name is invalid")
	}
	if !lowerSHA256Pattern.MatchString(sha256Hex) {
		return "", errors.New("ServiceDesk profile sha256 is invalid")
	}
	result := "profiles/" + name + "/" + sha256Hex + ".json"
	if err := ValidateRegistryPath(result); err != nil {
		return "", err
	}
	return result, nil
}

func ResolveRegistryURL(baseURL, relativePath string) (string, error) {
	if err := ValidateRegistryPath(relativePath); err != nil {
		return "", err
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !strings.HasSuffix(base.Path, "/") {
		return "", errors.New("ServiceDesk registry base URL must be an absolute HTTP URL ending with slash")
	}
	resolved, err := base.Parse(relativePath)
	if err != nil || resolved.Scheme != base.Scheme || resolved.Host != base.Host || !strings.HasPrefix(resolved.Path, base.Path) {
		return "", errors.New("ServiceDesk registry path escaped its base URL")
	}
	return resolved.String(), nil
}

type RegistryFetcher struct {
	HTTPClient *http.Client
	BaseURL    string
	NoCache    bool
}

type FetchedRegistry struct {
	IndexBytes  []byte
	IndexSHA256 string
	Registry    Registry
	Profiles    []FetchedProfile
}

type FetchedProfile struct {
	Entry   RegistryEntry
	URL     string
	Bytes   []byte
	Overlay Overlay
}

func (f RegistryFetcher) Fetch(ctx context.Context) (FetchedRegistry, error) {
	indexURL, err := ResolveRegistryURL(f.BaseURL, "index.json")
	if err != nil {
		return FetchedRegistry{}, err
	}
	indexBytes, err := f.fetch(ctx, indexURL)
	if err != nil {
		return FetchedRegistry{}, fmt.Errorf("fetch ServiceDesk registry index: %w", err)
	}
	registry, err := DecodeRegistry(indexBytes)
	if err != nil {
		return FetchedRegistry{}, err
	}
	result := FetchedRegistry{IndexBytes: indexBytes, IndexSHA256: ProfileSHA256(indexBytes), Registry: registry, Profiles: make([]FetchedProfile, 0, len(registry.Profiles))}
	for _, entry := range registry.Profiles {
		profileURL, err := ResolveRegistryURL(f.BaseURL, entry.Path)
		if err != nil {
			return FetchedRegistry{}, err
		}
		profileBytes, err := f.fetch(ctx, profileURL)
		if err != nil {
			return FetchedRegistry{}, fmt.Errorf("fetch ServiceDesk profile %q: %w", entry.Name, err)
		}
		overlay, err := ValidateRegistryProfile(entry, profileBytes)
		if err != nil {
			return FetchedRegistry{}, err
		}
		result.Profiles = append(result.Profiles, FetchedProfile{Entry: entry, URL: profileURL, Bytes: profileBytes, Overlay: overlay})
	}
	return result, nil
}

func (r FetchedRegistry) Profile(name string) (FetchedProfile, bool) {
	for _, profile := range r.Profiles {
		if profile.Entry.Name == name {
			return profile, true
		}
	}
	return FetchedProfile{}, false
}

func (f RegistryFetcher) fetch(ctx context.Context, target string) ([]byte, error) {
	client := f.HTTPClient
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		client = &http.Client{Timeout: 30 * time.Second, Transport: transport}
	} else {
		clone := *client
		client = &clone
		if client.Transport == nil {
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.Proxy = nil
			client.Transport = transport
		}
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if f.NoCache {
		request.Header.Set("Cache-Control", "no-cache")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP status %d", response.StatusCode)
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
