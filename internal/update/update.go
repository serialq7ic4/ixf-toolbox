package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const DefaultReleaseRepo = "serialq7ic4/ixf-toolbox"

var semverPattern = regexp.MustCompile(`^[vV]?(\d+)\.(\d+)\.(\d+)$`)

type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
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
	payload, err := CheckLatestRelease(repo, currentVersion, release)
	if err != nil {
		return nil, err
	}
	payload["applied"] = false
	payload["commands"] = []string{}
	if apply {
		return nil, fmt.Errorf("Go self-update apply is not implemented yet; run without --apply and use the installCommand")
	}
	return payload, nil
}

func installCommand(repo string, version string, available bool) string {
	if !available {
		return ""
	}
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	extension := ""
	if goos == "windows" {
		extension = ".exe"
	}
	artifact := fmt.Sprintf("ixf_%s_%s_%s%s", version, goos, goarch, extension)
	return fmt.Sprintf(
		"Download https://github.com/%s/releases/download/v%s/%s and replace the current ixf binary.",
		repo,
		version,
		artifact,
	)
}
