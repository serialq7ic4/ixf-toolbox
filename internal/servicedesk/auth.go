package servicedesk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	serviceDeskAPIKeyEnv      = "IXF_SERVICEDESK_API_KEY"
	serviceDeskBusinessIDEnv  = "IXF_SERVICEDESK_BUSINESS_ID"
	serviceDeskAPIKeyFile     = "servicedesk-api-key"
	serviceDeskBusinessIDFile = "servicedesk-business-id"
)

type Credential struct {
	APIKey string
	Source string
}

type CredentialLoader struct {
	LookupEnv func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
	Stat      func(string) (fs.FileInfo, error)
	GOOS      string
	HomeDir   string
}

func (l CredentialLoader) Load(explicitFile string) (Credential, error) {
	lookupEnv := l.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if value, ok := lookupEnv(serviceDeskAPIKeyEnv); ok {
		key := strings.TrimSpace(value)
		if key == "" {
			return Credential{}, errors.New("ServiceDesk credential from environment is empty")
		}
		return Credential{APIKey: key, Source: "environment"}, nil
	}

	path, err := l.credentialPath(explicitFile)
	if err != nil {
		return Credential{}, err
	}
	stat := l.Stat
	if stat == nil {
		stat = os.Stat
	}
	info, err := stat(path)
	if err != nil {
		return Credential{}, fmt.Errorf("inspect ServiceDesk credential file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Credential{}, fmt.Errorf("ServiceDesk credential file %q is not a regular file", path)
	}
	goos := l.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "windows" && info.Mode().Perm()&^fs.FileMode(0o600) != 0 {
		return Credential{}, fmt.Errorf("ServiceDesk credential file %q permissions are wider than 0600", path)
	}

	readFile := l.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	raw, err := readFile(path)
	if err != nil {
		return Credential{}, fmt.Errorf("read ServiceDesk credential file %q: %w", path, err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		return Credential{}, fmt.Errorf("ServiceDesk credential file %q is empty", path)
	}
	return Credential{APIKey: key, Source: path}, nil
}

func (l CredentialLoader) credentialPath(explicitFile string) (string, error) {
	if explicitFile != "" {
		return explicitFile, nil
	}
	home, err := loaderHomeDir(l.HomeDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ixf-toolbox", serviceDeskAPIKeyFile), nil
}

type BusinessIDLoader struct {
	LookupEnv func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
	HomeDir   string
}

func (l BusinessIDLoader) Load(explicit int) (id int, source string, configured bool, err error) {
	if explicit != 0 {
		if explicit < 0 {
			return 0, "", false, errors.New("ServiceDesk business ID must be a positive integer")
		}
		return explicit, "flag", true, nil
	}

	lookupEnv := l.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if value, ok := lookupEnv(serviceDeskBusinessIDEnv); ok {
		id, err := parsePositiveID(strings.TrimSpace(value))
		if err != nil {
			return 0, "", false, fmt.Errorf("invalid ServiceDesk business ID in environment: %w", err)
		}
		return id, "environment", true, nil
	}

	home, err := loaderHomeDir(l.HomeDir)
	if err != nil {
		return 0, "", false, err
	}
	path := filepath.Join(home, ".config", "ixf-toolbox", serviceDeskBusinessIDFile)
	readFile := l.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	raw, err := readFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("read ServiceDesk business ID file %q: %w", path, err)
	}
	id, err = parsePositiveID(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, "", false, fmt.Errorf("invalid ServiceDesk business ID file %q: %w", path, err)
	}
	return id, path, true, nil
}

func loaderHomeDir(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for ServiceDesk configuration: %w", err)
	}
	return home, nil
}

func parsePositiveID(raw string) (int, error) {
	if raw == "" {
		return 0, errors.New("value is empty")
	}
	for _, char := range raw {
		if char < '0' || char > '9' {
			return 0, errors.New("value is not a positive integer")
		}
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, errors.New("value is not a positive integer")
	}
	return id, nil
}
