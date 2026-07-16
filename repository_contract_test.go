package ixftoolbox

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryIsGoOnlyAfterPythonHarnessRemoval(t *testing.T) {
	root := repoRoot(t)
	var pythonFiles []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".pytest_cache", ".ruff_cache", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".py") {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			pythonFiles = append(pythonFiles, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	if len(pythonFiles) != 0 {
		t.Fatalf("python source files remain after Go-only migration: %v", pythonFiles)
	}
}

func TestWorkflowsUseGoToolchainOnly(t *testing.T) {
	for _, relative := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml"} {
		text := readRepoFile(t, relative)
		for _, forbidden := range []string{
			"actions/setup-python",
			"python -m pytest",
			"python -m ruff",
			"python scripts/",
			"pytest",
			"ruff",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s still references %q", relative, forbidden)
			}
		}
		for _, expected := range []string{"actions/setup-go", "go test ./...", "go vet ./..."} {
			if !strings.Contains(text, expected) {
				t.Fatalf("%s missing %q", relative, expected)
			}
		}
	}
}

func TestReadmeDescribesNaturalAgentPromptsAndBackgroundRouting(t *testing.T) {
	text := readRepoFile(t, "README.md")
	for _, forbidden := range []string{
		"请用 using-ixf-toolbox 判断",
		"请用 ixf-docs-reader",
		"请用 ixf-docs-writer",
		"请用 ixf-okr-reader",
		"请用 ixf-okr-writer",
		"Python 仍可作为本仓库的测试 harness 使用",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("README.md still contains agent-facing implementation detail %q", forbidden)
		}
	}
	for _, expected := range []string{
		"直接按日常方式描述目标即可",
		"using-ixf-toolbox 会在后台识别",
		"帮我总结一下这个文档",
		"把我确认后的 O3 和 3 个 KR 写入这个 OKR 页面",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("README.md missing natural usage text %q", expected)
		}
	}
}

func TestReleaseNotesCanBeExtractedWithoutPython(t *testing.T) {
	changelog := readRepoFile(t, "CHANGELOG.md")
	section := extractChangelogSection(changelog, "3.1.0")
	for _, expected := range []string{
		"Removed the Python pytest harness",
		"Updated README agent usage examples",
	} {
		if !strings.Contains(section, expected) {
			t.Fatalf("v3.1.0 changelog section missing %q:\n%s", expected, section)
		}
	}
	if strings.Contains(section, "## 3.0.0") {
		t.Fatalf("v3.1.0 changelog section included the next version:\n%s", section)
	}
}

func extractChangelogSection(markdown string, version string) string {
	header := "## " + version
	start := strings.Index(markdown, header)
	if start < 0 {
		return ""
	}
	bodyStart := strings.Index(markdown[start:], "\n")
	if bodyStart < 0 {
		return ""
	}
	body := markdown[start+bodyStart+1:]
	next := strings.Index(body, "\n## ")
	if next >= 0 {
		body = body[:next]
	}
	return strings.TrimSpace(body)
}

func readRepoFile(t *testing.T, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return string(content)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
