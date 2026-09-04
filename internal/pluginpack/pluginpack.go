package pluginpack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Options struct {
	Root  string
	Check bool
}

var canonicalSkillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
	"ixf-messenger-reader",
	"ixf-messenger-writer",
}

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type metadata struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Author      author      `json:"author"`
	Homepage    string      `json:"homepage"`
	Repository  string      `json:"repository"`
	License     string      `json:"license"`
	Keywords    []string    `json:"keywords"`
	Interface   interfaceUI `json:"interface"`
}

type author struct {
	Name string `json:"name"`
}

type interfaceUI struct {
	DisplayName      string   `json:"displayName"`
	ShortDescription string   `json:"shortDescription"`
	LongDescription  string   `json:"longDescription"`
	DeveloperName    string   `json:"developerName"`
	Category         string   `json:"category"`
	Capabilities     []string `json:"capabilities"`
	DefaultPrompt    []string `json:"defaultPrompt"`
}

type codexManifest struct {
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Description string      `json:"description"`
	Author      author      `json:"author"`
	Homepage    string      `json:"homepage"`
	Repository  string      `json:"repository"`
	License     string      `json:"license"`
	Keywords    []string    `json:"keywords"`
	Interface   interfaceUI `json:"interface"`
	Skills      string      `json:"skills"`
}

type claudeManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      author   `json:"author"`
	Homepage    string   `json:"homepage"`
	Repository  string   `json:"repository"`
	License     string   `json:"license"`
	Keywords    []string `json:"keywords"`
}

type codexMarketplace struct {
	Name    string                   `json:"name"`
	Owner   author                   `json:"owner"`
	Plugins []codexMarketplacePlugin `json:"plugins"`
}

type codexMarketplacePlugin struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Source      codexPluginSource `json:"source"`
	Category    string            `json:"category"`
	Policy      codexPluginPolicy `json:"policy"`
}

type codexPluginSource struct {
	Path string `json:"path"`
}

type codexPluginPolicy struct {
	Installation   string `json:"installation"`
	Authentication string `json:"authentication"`
}

type claudeMarketplace struct {
	Schema  string                    `json:"$schema"`
	Name    string                    `json:"name"`
	Owner   author                    `json:"owner"`
	Plugins []claudeMarketplacePlugin `json:"plugins"`
}

type claudeMarketplacePlugin struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Category    string `json:"category"`
}

type fileEntry struct {
	Mode os.FileMode
	Data []byte
	Link string
}

func Generate(options Options) error {
	root := options.Root
	if root == "" {
		return errors.New("plugin package root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve plugin package root: %w", err)
	}
	version, err := readVersion(filepath.Join(root, "VERSION"))
	if err != nil {
		return err
	}
	var info metadata
	metadataBytes, err := os.ReadFile(filepath.Join(root, "plugin-src", "metadata.json"))
	if err != nil {
		return fmt.Errorf("read plugin metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(metadataBytes))
	if err := decoder.Decode(&info); err != nil {
		return fmt.Errorf("decode plugin metadata: %w", err)
	}
	if err := validateMetadata(info); err != nil {
		return err
	}

	buildRoot, err := os.MkdirTemp(root, ".pluginpack-")
	if err != nil {
		return fmt.Errorf("create plugin package staging directory: %w", err)
	}
	defer os.RemoveAll(buildRoot)
	if err := buildPackages(root, buildRoot, version, info); err != nil {
		return err
	}

	generatedRoots := []string{
		".agents/plugins/marketplace.json",
		".claude-plugin/marketplace.json",
		"plugins/codex/ixf-toolbox",
		"plugins/claude/ixf-toolbox",
	}
	if options.Check {
		drift, err := compareGeneratedRoots(buildRoot, root, generatedRoots)
		if err != nil {
			return err
		}
		if len(drift) > 0 {
			return fmt.Errorf("generated plugin packages are stale: %s", strings.Join(drift, ", "))
		}
		return nil
	}
	for _, relative := range generatedRoots {
		if err := replaceGeneratedRoot(root, buildRoot, relative); err != nil {
			return err
		}
	}
	return nil
}

func readVersion(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read VERSION: %w", err)
	}
	version := strings.TrimSpace(string(content))
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("VERSION must be strict X.Y.Z, got %q", version)
	}
	return version, nil
}

func validateMetadata(info metadata) error {
	for name, value := range map[string]string{
		"name":                       info.Name,
		"description":                info.Description,
		"author.name":                info.Author.Name,
		"homepage":                   info.Homepage,
		"repository":                 info.Repository,
		"license":                    info.License,
		"interface.displayName":      info.Interface.DisplayName,
		"interface.shortDescription": info.Interface.ShortDescription,
		"interface.longDescription":  info.Interface.LongDescription,
		"interface.developerName":    info.Interface.DeveloperName,
		"interface.category":         info.Interface.Category,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("plugin metadata field %s is required", name)
		}
	}
	if info.Name != "ixf-toolbox" {
		return fmt.Errorf("plugin metadata name must be ixf-toolbox, got %q", info.Name)
	}
	if len(info.Interface.Capabilities) == 0 || len(info.Interface.DefaultPrompt) == 0 {
		return errors.New("plugin metadata interface capabilities and defaultPrompt are required")
	}
	return nil
}

func buildPackages(root, buildRoot, version string, info metadata) error {
	for _, name := range canonicalSkillNames {
		source := filepath.Join(root, "skills", name)
		stat, err := os.Lstat(source)
		if err != nil {
			return fmt.Errorf("required canonical skill %s: %w", name, err)
		}
		if stat.Mode()&os.ModeSymlink != 0 || !stat.IsDir() {
			return fmt.Errorf("canonical skill %s must be a directory without symlink", name)
		}
		for _, host := range []string{"codex", "claude"} {
			destination := filepath.Join(buildRoot, "plugins", host, "ixf-toolbox", "skills", name)
			if err := copyRegularTree(source, destination); err != nil {
				return fmt.Errorf("copy canonical skill %s: %w", name, err)
			}
		}
	}

	codex := codexManifest{
		Name: info.Name, Version: version, Description: info.Description, Author: info.Author,
		Homepage: info.Homepage, Repository: info.Repository, License: info.License,
		Keywords: info.Keywords, Interface: info.Interface, Skills: "./skills/",
	}
	claude := claudeManifest{
		Name: info.Name, Version: version, Description: info.Description, Author: info.Author,
		Homepage: info.Homepage, Repository: info.Repository, License: info.License, Keywords: info.Keywords,
	}
	if err := writeJSON(filepath.Join(buildRoot, "plugins/codex/ixf-toolbox/.codex-plugin/plugin.json"), codex); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(buildRoot, "plugins/claude/ixf-toolbox/.claude-plugin/plugin.json"), claude); err != nil {
		return err
	}

	codexMarket := codexMarketplace{
		Name: info.Name, Owner: info.Author,
		Plugins: []codexMarketplacePlugin{{
			Name: info.Name, Description: info.Description,
			Source: codexPluginSource{Path: "./plugins/codex/ixf-toolbox"}, Category: info.Interface.Category,
			Policy: codexPluginPolicy{Installation: "AVAILABLE", Authentication: "ON_USE"},
		}},
	}
	claudeMarket := claudeMarketplace{
		Schema: "https://anthropic.com/claude-code/marketplace.schema.json", Name: info.Name, Owner: info.Author,
		Plugins: []claudeMarketplacePlugin{{
			Name: info.Name, Description: info.Description, Source: "./plugins/claude/ixf-toolbox", Category: info.Interface.Category,
		}},
	}
	if err := writeJSON(filepath.Join(buildRoot, ".agents/plugins/marketplace.json"), codexMarket); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(buildRoot, ".claude-plugin/marketplace.json"), claudeMarket); err != nil {
		return err
	}
	return nil
}

func copyRegularTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := destination
		if relative != "." {
			target = filepath.Join(destination, relative)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not allowed: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("only regular files are allowed: %s", path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
}

func writeJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func replaceGeneratedRoot(root, buildRoot, relative string) error {
	destination := filepath.Join(root, filepath.FromSlash(relative))
	source := filepath.Join(buildRoot, filepath.FromSlash(relative))
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("replace generated root %s: %w", relative, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stage generated root %s: %w", relative, err)
	}
	if info.IsDir() {
		if err := copyRegularTree(source, destination); err != nil {
			return fmt.Errorf("write generated root %s: %w", relative, err)
		}
		return nil
	}
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, content, info.Mode().Perm())
}

func compareGeneratedRoots(buildRoot, root string, relatives []string) ([]string, error) {
	var drift []string
	for _, relative := range relatives {
		want, err := snapshot(filepath.Join(buildRoot, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("snapshot staged root %s: %w", relative, err)
		}
		got, err := snapshot(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, fmt.Errorf("snapshot generated root %s: %w", relative, err)
		}
		paths := map[string]bool{}
		for path := range want {
			paths[path] = true
		}
		for path := range got {
			paths[path] = true
		}
		for path := range paths {
			if !sameFileEntry(want[path], got[path]) {
				drift = append(drift, filepath.ToSlash(filepath.Join(relative, path)))
			}
		}
	}
	sort.Strings(drift)
	return drift, nil
}

func snapshot(root string) (map[string]fileEntry, error) {
	result := map[string]fileEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == root {
			return err
		}
		if err != nil {
			return err
		}
		if path == root && entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			result[filepath.ToSlash(relative)] = fileEntry{Mode: entry.Type(), Link: link}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("generated root contains unsupported file %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = fileEntry{Mode: info.Mode().Perm(), Data: data}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	return result, err
}

func sameFileEntry(want, got fileEntry) bool {
	return want.Mode == got.Mode && want.Link == got.Link && bytes.Equal(want.Data, got.Data)
}
