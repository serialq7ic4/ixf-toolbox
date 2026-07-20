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
		"看一下未读消息",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("README.md missing natural usage text %q", expected)
		}
	}
}

func TestCurrentAgentGuidanceForbidsPythonAndLegacyFallbacks(t *testing.T) {
	agentGuidance := readRepoFile(t, "AGENTS.md")
	for _, expected := range []string{
		"Go-only runtime",
		"Do not use Python fallback",
		"Do not call `ixfdoc` or `ixfwrite`",
		"docs/superpowers/",
		"historical",
	} {
		if !strings.Contains(agentGuidance, expected) {
			t.Fatalf("AGENTS.md missing %q:\n%s", expected, agentGuidance)
		}
	}

	for _, relative := range []string{
		"README.md",
		"README.en.md",
		"docs/go-python-parity.md",
		"skills_embed.go",
	} {
		text := readRepoFile(t, relative)
		for _, forbidden := range []string{
			"same agent skills installed by the Python runtime",
			"Python reference runtime still handles",
			"Python-compatible reader",
			"Python fallback for wiki",
			"ixfdoc fallback",
			"ixfwrite fallback",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains misleading current-runtime guidance %q:\n%s", relative, forbidden, text)
			}
		}
	}
}

func TestAgentRoutingContractIsAuthoritativeAndNatural(t *testing.T) {
	routingDoc := readRepoFile(t, "docs/agent-routing.md")
	for _, expected := range []string{
		"Authoritative Current Guidance",
		"Users do not need to name skills explicitly",
		"background routing",
		"Default ambiguous intent to read-only",
		"Do not use `docs/superpowers/`",
		"`ixf doctor --json` exposes `agentRouting`",
	} {
		if !strings.Contains(routingDoc, expected) {
			t.Fatalf("docs/agent-routing.md missing %q:\n%s", expected, routingDoc)
		}
	}

	for _, runtimeDir := range []string{"skills/codex", "skills/claude-code"} {
		routing := readRepoFile(t, filepath.ToSlash(filepath.Join(runtimeDir, "using-ixf-toolbox", "SKILL.md")))
		for _, expected := range []string{
			"Users do not need to name this skill",
			"background routing",
			"Default ambiguous intent to read-only",
			"docs/agent-routing.md",
			"Do not route from historical implementation notes",
		} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s routing skill missing %q:\n%s", runtimeDir, expected, routing)
			}
		}
	}
}

func TestIxfSkillsRejectPythonAndLegacyFallbacks(t *testing.T) {
	for _, runtimeDir := range []string{"skills/codex", "skills/claude-code"} {
		for _, skillName := range skillNamesForContract() {
			text := readRepoFile(t, filepath.ToSlash(filepath.Join(runtimeDir, skillName, "SKILL.md")))
			for _, expected := range []string{"Go `ixf` only", "Do not call `ixfdoc` or `ixfwrite`", "Do not use Python fallback"} {
				if !strings.Contains(text, expected) {
					t.Fatalf("%s/%s missing no-legacy rule %q:\n%s", runtimeDir, skillName, expected, text)
				}
			}
		}
	}
}

func TestMessengerSkillsAreRoutedAndDocumentDryRunSafety(t *testing.T) {
	for _, runtimeDir := range []string{"skills/codex", "skills/claude-code"} {
		routing := readRepoFile(t, filepath.ToSlash(filepath.Join(runtimeDir, "using-ixf-toolbox", "SKILL.md")))
		for _, expected := range []string{"ixf-messenger-reader", "ixf-messenger-writer", "Default to read-only"} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s routing skill missing %q:\n%s", runtimeDir, expected, routing)
			}
		}

		reader := readRepoFile(t, filepath.ToSlash(filepath.Join(runtimeDir, "ixf-messenger-reader", "SKILL.md")))
		for _, expected := range []string{"name: ixf-messenger-reader", "ixf messenger doctor --json", "ixf messenger read", "read-only", "--apply", "never sends", "Chrome/Chromium-only", "may mark opened chats as read"} {
			if !strings.Contains(reader, expected) {
				t.Fatalf("%s messenger reader missing %q:\n%s", runtimeDir, expected, reader)
			}
		}

		writer := readRepoFile(t, filepath.ToSlash(filepath.Join(runtimeDir, "ixf-messenger-writer", "SKILL.md")))
		for _, expected := range []string{"name: ixf-messenger-writer", "ixf messenger send", "dry-run", "--apply", "fresh-session verification", "targetVerified:true", "localEchoMatched:true", "verifiedPresent:true"} {
			if !strings.Contains(writer, expected) {
				t.Fatalf("%s messenger writer missing %q:\n%s", runtimeDir, expected, writer)
			}
		}
	}
}

func TestDocsWriterSkillDoesNotOverclaimExistingDocumentUpdate(t *testing.T) {
	for _, runtimeDir := range []string{"skills/codex", "skills/claude-code"} {
		writerPath := filepath.ToSlash(filepath.Join(runtimeDir, "ixf-docs-writer", "SKILL.md"))
		writer := readRepoFile(t, writerPath)
		for _, expected := range []string{
			"create-only",
			"new docx",
			"does not modify existing docx",
			"Use `ixf docs publish`",
			"Use `ixf docs update`",
			"replace_body",
			"explicit approval",
		} {
			if !strings.Contains(writer, expected) {
				t.Fatalf("%s missing create-only boundary %q:\n%s", writerPath, expected, writer)
			}
		}
		for _, forbidden := range []string{
			"modifying document content",
			"create or modify content",
			"document modification",
		} {
			if strings.Contains(writer, forbidden) {
				t.Fatalf("%s still overclaims existing-doc update with %q:\n%s", writerPath, forbidden, writer)
			}
		}

		routingPath := filepath.ToSlash(filepath.Join(runtimeDir, "using-ixf-toolbox", "SKILL.md"))
		routing := readRepoFile(t, routingPath)
		for _, expected := range []string{
			"approved Markdown publishing as a new docx document",
			"existing-docx update",
		} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s missing docs writer routing boundary %q:\n%s", routingPath, expected, routing)
			}
		}
	}
}

func skillNamesForContract() []string {
	return []string{
		"using-ixf-toolbox",
		"ixf-docs-reader",
		"ixf-docs-writer",
		"ixf-okr-reader",
		"ixf-okr-writer",
		"ixf-messenger-reader",
		"ixf-messenger-writer",
	}
}

func TestMessengerGADocumentationCoversOperationalBoundaries(t *testing.T) {
	messengerDoc := readRepoFile(t, "docs/messenger.md")
	for _, expected := range []string{
		"Chrome/Chromium-only",
		"profile_explorer",
		"cloned profile",
		"read/open may mark chats as read",
		"targetVerified:true",
		"localEchoMatched:true",
		"verifiedPresent:true",
		"ixf messenger doctor --json",
	} {
		if !strings.Contains(messengerDoc, expected) {
			t.Fatalf("docs/messenger.md missing %q:\n%s", expected, messengerDoc)
		}
	}

	for _, relative := range []string{"README.md", "README.en.md", "docs/supported-platforms.md"} {
		text := readRepoFile(t, relative)
		for _, expected := range []string{"docs/messenger.md", "Chrome/Chromium", "cloned profile"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("%s missing %q:\n%s", relative, expected, text)
			}
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
