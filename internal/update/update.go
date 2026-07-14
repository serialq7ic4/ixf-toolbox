package update

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const DefaultReleaseRepo = "serialq7ic4/ixf-toolbox"

var semverPattern = regexp.MustCompile(`^[vV]?(\d+)\.(\d+)\.(\d+)$`)

type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

type SelfUpdateOptions struct {
	Repo           string
	CurrentVersion string
	Release        Release
	Apply          bool
	TargetPath     string
}

func NormalizeVersion(value string) ([3]int, error) {
	match := semverPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return [3]int{}, fmt.Errorf("invalid semantic version: %s", value)
	}
	parts := [3]int{}
	for i := range parts {
		number, err := strconv.Atoi(match[i+1])
		if err != nil {
			return [3]int{}, err
		}
		parts[i] = number
	}
	return parts, nil
}

func VersionString(value string) (string, error) {
	parts, err := NormalizeVersion(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d", parts[0], parts[1], parts[2]), nil
}

func IsNewerVersion(current string, candidate string) (bool, error) {
	currentParts, err := NormalizeVersion(current)
	if err != nil {
		return false, err
	}
	candidateParts, err := NormalizeVersion(candidate)
	if err != nil {
		return false, err
	}
	return candidateParts[0] > currentParts[0] ||
		(candidateParts[0] == currentParts[0] && candidateParts[1] > currentParts[1]) ||
		(candidateParts[0] == currentParts[0] && candidateParts[1] == currentParts[1] && candidateParts[2] > currentParts[2]), nil
}

func LoadRelease(repo string, releaseFile string) (Release, error) {
	if releaseFile != "" {
		content, err := os.ReadFile(releaseFile)
		if err != nil {
			return Release{}, err
		}
		var release Release
		if err := json.Unmarshal(content, &release); err != nil {
			return Release{}, err
		}
		return release, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := http.Client{Timeout: 15 * time.Second}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Release{}, fmt.Errorf("GitHub release request failed: %s", response.Status)
	}
	var release Release
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return Release{}, err
	}
	return release, nil
}

func CheckLatestRelease(repo string, currentVersion string, release Release) (map[string]any, error) {
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("GitHub release response did not include tag_name")
	}
	current, err := VersionString(currentVersion)
	if err != nil {
		return nil, err
	}
	latest, err := VersionString(release.TagName)
	if err != nil {
		return nil, err
	}
	available, err := IsNewerVersion(current, latest)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":              true,
		"currentVersion":  current,
		"latestVersion":   latest,
		"latestTag":       "v" + latest,
		"updateAvailable": available,
		"releaseUrl":      release.HTMLURL,
		"installCommand":  installCommand(repo, latest, available),
	}, nil
}

func SelfUpdatePayload(repo string, currentVersion string, release Release, apply bool) (map[string]any, error) {
	return SelfUpdateWithOptions(SelfUpdateOptions{
		Repo:           repo,
		CurrentVersion: currentVersion,
		Release:        release,
		Apply:          apply,
	})
}

func SelfUpdateWithOptions(options SelfUpdateOptions) (map[string]any, error) {
	repo := options.Repo
	if repo == "" {
		repo = DefaultReleaseRepo
	}
	payload, err := CheckLatestRelease(repo, options.CurrentVersion, options.Release)
	if err != nil {
		return nil, err
	}
	payload["applied"] = false
	payload["commands"] = []string{}
	latest, _ := payload["latestVersion"].(string)
	artifactName := ArtifactName(latest, runtime.GOOS, runtime.GOARCH)
	checksumName := ChecksumName(latest)
	payload["artifactName"] = artifactName
	payload["checksumName"] = checksumName
	payload["checksumVerified"] = false
	if updateAvailable, _ := payload["updateAvailable"].(bool); !updateAvailable || !options.Apply {
		return payload, nil
	}
	targetPath := options.TargetPath
	if targetPath == "" {
		targetPath, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	artifactURL, err := assetDownloadURL(options.Release.Assets, artifactName)
	if err != nil {
		return nil, err
	}
	checksumURL, err := assetDownloadURL(options.Release.Assets, checksumName)
	if err != nil {
		return nil, err
	}
	checksumBytes, err := downloadBytes(checksumURL)
	if err != nil {
		return nil, err
	}
	expected, err := expectedChecksum(string(checksumBytes), artifactName)
	if err != nil {
		return nil, err
	}
	artifactBytes, err := downloadBytes(artifactURL)
	if err != nil {
		return nil, err
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(artifactBytes))
	if !strings.EqualFold(actual, expected) {
		return nil, fmt.Errorf("checksum mismatch for %s", artifactName)
	}
	if err := replaceTarget(targetPath, artifactBytes); err != nil {
		return nil, err
	}
	payload["applied"] = true
	payload["checksumVerified"] = true
	payload["targetPath"] = targetPath
	return payload, nil
}

func ArtifactName(version string, goos string, goarch string) string {
	artifact := fmt.Sprintf("ixf_%s_%s_%s", version, goos, goarch)
	if goos == "windows" {
		artifact += ".exe"
	}
	return artifact
}

func ChecksumName(version string) string {
	return fmt.Sprintf("ixf_%s_checksums.txt", version)
}

func installCommand(repo string, version string, available bool) string {
	if !available {
		return ""
	}
	artifact := ArtifactName(version, runtime.GOOS, runtime.GOARCH)
	return fmt.Sprintf(
		"Download https://github.com/%s/releases/download/v%s/%s and replace the current ixf binary.",
		repo,
		version,
		artifact,
	)
}

func assetDownloadURL(assets []Asset, name string) (string, error) {
	for _, asset := range assets {
		if asset.Name != name {
			continue
		}
		if asset.BrowserDownloadURL != "" {
			return asset.BrowserDownloadURL, nil
		}
		if asset.URL != "" {
			return asset.URL, nil
		}
	}
	return "", fmt.Errorf("release asset not found: %s", name)
}

func downloadBytes(location string) ([]byte, error) {
	parsed, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "file" {
		path, err := fileURLPath(parsed)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	}
	client := http.Client{Timeout: 60 * time.Second}
	response, err := client.Get(location)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("download failed: %s", response.Status)
	}
	return io.ReadAll(response.Body)
}

func fileURLPath(parsed *url.URL) (string, error) {
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(path, "/") && len(path) >= 3 && path[2] == ':' {
		path = path[1:]
	}
	return path, nil
}

func expectedChecksum(text string, artifactName string) (string, error) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if filepath.Base(fields[len(fields)-1]) == artifactName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum entry not found for %s", artifactName)
}

func replaceTarget(targetPath string, content []byte) error {
	mode := os.FileMode(0o755)
	if info, err := os.Stat(targetPath); err == nil {
		mode = info.Mode().Perm() | 0o111
	}
	dir := filepath.Dir(targetPath)
	temp, err := os.CreateTemp(dir, filepath.Base(targetPath)+".tmp-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(targetPath)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		if removeErr := os.Remove(targetPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if retryErr := os.Rename(tempPath, targetPath); retryErr != nil {
			return retryErr
		}
	}
	cleanup = false
	return nil
}
