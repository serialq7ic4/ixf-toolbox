package ixftoolbox

import (
	"bytes"
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

func TestVersionIsOwnedByVersionFileNotLdflags(t *testing.T) {
	for _, relative := range []string{
		".github/workflows/release.yml",
		".github/PULL_REQUEST_TEMPLATE.md",
		"CONTRIBUTING.md",
		"README.md",
		"README.en.md",
	} {
		text := readRepoFile(t, relative)
		if strings.Contains(text, "-X main.version") {
			t.Fatalf("%s still documents ldflags version injection", relative)
		}
	}

	embedSource := readRepoFile(t, "version_embed.go")
	if strings.Contains(embedSource, "override main.version") {
		t.Fatalf("version_embed.go still describes release ldflags as the version source")
	}
	if _, err := os.Stat(filepath.Join(repoRoot(t), "skills_embed.go")); !os.IsNotExist(err) {
		t.Fatalf("skills_embed.go must be absent, stat error = %v", err)
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
		"version_embed.go",
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

	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		routingPath := filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md"))
		routing := readRepoFile(t, routingPath)
		for _, expected := range []string{
			"Users do not need to name this skill",
			"background routing",
			"Default ambiguous intent to read-only",
			"docs/agent-routing.md",
			"Do not route from historical implementation notes",
		} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s routing skill missing %q:\n%s", routingPath, expected, routing)
			}
		}
	}
}

var routingTriggers = []string{
	"i讯飞", "讯飞文档", "LarkShell", "/docx/", "/wiki/", "/base/",
	"docx", "wiki", "sheets", "bitable", "OKR", "Messenger",
	"read", "publish", "update", "patch", "append row", "upload image", "attachment",
}

func TestRoutingSkillTriggersNativePluginDiscovery(t *testing.T) {
	const frontmatter = "description: Use when a request mentions i讯飞, 讯飞文档, or LarkShell and involves /docx/, /wiki/, /base/, docx, wiki, sheets, bitable, OKR, or Messenger, including read, publish, update, patch, append-row, image upload, attachment upload, or message workflows; do not use for ordinary local Markdown reading or editing."
	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		path := filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md"))
		content := readRepoFile(t, path)
		if !strings.Contains(content, frontmatter) {
			t.Fatalf("%s missing native plugin routing frontmatter:\n%s", path, content)
		}
		for _, trigger := range routingTriggers {
			if !strings.Contains(content, trigger) {
				t.Fatalf("%s missing routing trigger %q:\n%s", path, trigger, content)
			}
		}
	}
}

func TestSkillRuntimeBootstrapContract(t *testing.T) {
	required := []string{
		"Resolve the Go `ixf` runtime before running a business command.",
		"Do not download or install anything without explicit user confirmation.",
		"Run the packaged bootstrap with `--dry-run` before `--apply`.",
		"Run `ixf doctor --json` after bootstrap succeeds.",
		"Use `ixf deps install`, not bootstrap, for Mermaid dependencies.",
		"filepath.Dir(filepath.Dir(filepath.Dir(skillFile)))",
		"runtime.json",
		"scripts/bootstrap-runtime.sh",
		"scripts/bootstrap-runtime.ps1",
		"minimumVersion",
		"~/.local/share/ixf-toolbox/bin/ixf",
		`%LOCALAPPDATA%\ixf-toolbox\bin\ixf.exe`,
	}
	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		for _, name := range canonicalSkillNames {
			path := filepath.ToSlash(filepath.Join(skillRoot, name, "SKILL.md"))
			content := readRepoFile(t, path)
			for _, expected := range required {
				if !strings.Contains(content, expected) {
					t.Fatalf("%s missing runtime bootstrap contract %q:\n%s", path, expected, content)
				}
			}
		}
	}
}

func TestLocalMarkdownDoesNotDefaultToIxfDocsReader(t *testing.T) {
	routingDoc := readRepoFile(t, "docs/agent-routing.md")
	normalizedRoutingDoc := strings.Join(strings.Fields(routingDoc), " ")
	for _, expected := range []string{
		"Ordinary local Markdown files do not require ixf Toolbox",
		"Intent, not file type, is the routing trigger",
		"use the host filesystem",
		"explicit chunking, artifact generation, publish, update, or patch workflow",
		"deterministic artifact or manifest output",
	} {
		if !strings.Contains(normalizedRoutingDoc, expected) {
			t.Fatalf("docs/agent-routing.md missing local Markdown routing boundary %q:\n%s", expected, routingDoc)
		}
	}

	readme := readRepoFile(t, "README.md")
	for _, expected := range []string{
		"触发条件是意图，不是 `.md` 文件类型",
		"稳定的 artifact / manifest",
		"heading-aware outline/chunk",
	} {
		if !strings.Contains(readme, expected) {
			t.Fatalf("README.md missing local Markdown rationale %q:\n%s", expected, readme)
		}
	}

	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		routingPath := filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md"))
		routing := readRepoFile(t, routingPath)
		for _, forbidden := range []string{"Use this skill for local Markdown reading", "local Markdown sources"} {
			if strings.Contains(routing, forbidden) {
				t.Fatalf("%s still routes ordinary local Markdown through ixf docs reader with %q:\n%s", routingPath, forbidden, routing)
			}
		}
		for _, expected := range []string{"Ordinary local Markdown files do not require ixf Toolbox", "use the host filesystem"} {
			if !strings.Contains(strings.Join(strings.Fields(routing), " "), expected) {
				t.Fatalf("%s missing local Markdown filesystem boundary %q:\n%s", routingPath, expected, routing)
			}
		}

		readerPath := filepath.ToSlash(filepath.Join(skillRoot, "ixf-docs-reader", "SKILL.md"))
		reader := readRepoFile(t, readerPath)
		for _, forbidden := range []string{"local Markdown sources", "local Markdown files into local artifacts"} {
			if strings.Contains(reader, forbidden) {
				t.Fatalf("%s still claims ordinary local Markdown reader routing with %q:\n%s", readerPath, forbidden, reader)
			}
		}
		for _, expected := range []string{"Ordinary local Markdown files do not require this skill", "Use the host filesystem"} {
			if !strings.Contains(strings.Join(strings.Fields(reader), " "), expected) {
				t.Fatalf("%s missing local Markdown filesystem guidance %q:\n%s", readerPath, expected, reader)
			}
		}
	}
}

func TestIxfSkillsRejectPythonAndLegacyFallbacks(t *testing.T) {
	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		for _, skillName := range skillNamesForContract() {
			path := filepath.ToSlash(filepath.Join(skillRoot, skillName, "SKILL.md"))
			text := readRepoFile(t, path)
			for _, expected := range []string{"Go `ixf` only", "Do not call `ixfdoc` or `ixfwrite`", "Do not use Python fallback"} {
				if !strings.Contains(text, expected) {
					t.Fatalf("%s missing no-legacy rule %q:\n%s", path, expected, text)
				}
			}
		}
	}
}

func TestMessengerSkillsAreRoutedAndDocumentDryRunSafety(t *testing.T) {
	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		routing := readRepoFile(t, filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md")))
		for _, expected := range []string{"ixf-messenger-reader", "ixf-messenger-writer", "Default to read-only"} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s routing skill missing %q:\n%s", skillRoot, expected, routing)
			}
		}

		reader := readRepoFile(t, filepath.ToSlash(filepath.Join(skillRoot, "ixf-messenger-reader", "SKILL.md")))
		for _, expected := range []string{"name: ixf-messenger-reader", "ixf messenger doctor --json", "ixf messenger read", "read-only", "--apply", "never sends", "Chrome/Chromium-only", "may mark opened chats as read"} {
			if !strings.Contains(reader, expected) {
				t.Fatalf("%s messenger reader missing %q:\n%s", skillRoot, expected, reader)
			}
		}

		writer := readRepoFile(t, filepath.ToSlash(filepath.Join(skillRoot, "ixf-messenger-writer", "SKILL.md")))
		for _, expected := range []string{"name: ixf-messenger-writer", "ixf messenger send", "dry-run", "--apply", "fresh-session verification", "targetVerified:true", "localEchoMatched:true", "verifiedPresent:true"} {
			if !strings.Contains(writer, expected) {
				t.Fatalf("%s messenger writer missing %q:\n%s", skillRoot, expected, writer)
			}
		}
	}
}

func TestDocsWriterSkillDoesNotOverclaimExistingDocumentUpdate(t *testing.T) {
	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		writerPath := filepath.ToSlash(filepath.Join(skillRoot, "ixf-docs-writer", "SKILL.md"))
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

		routingPath := filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md"))
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

func TestDocsWriterSkillRoutesLocalizedInsertToPatch(t *testing.T) {
	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		writerPath := filepath.ToSlash(filepath.Join(skillRoot, "ixf-docs-writer", "SKILL.md"))
		writer := readRepoFile(t, writerPath)
		for _, expected := range []string{
			"ixf docs patch insert",
			"insert under heading",
			"do not use `ixf docs update` for localized insertion",
			"duplicateCandidate",
			"verify.unchangedExistingBlocks",
			"ixf docs patch replace-section",
			"ixf docs patch delete-section",
			"verify.unchangedOutsideSectionBlocks",
		} {
			if !strings.Contains(writer, expected) {
				t.Fatalf("%s missing localized insert routing %q:\n%s", writerPath, expected, writer)
			}
		}
	}
}

func TestDocsUpdateRunbookDocumentsStableSafetyBoundary(t *testing.T) {
	runbook := readRepoFile(t, "docs/docs-update.md")
	for _, expected := range []string{
		"ixf docs update",
		"replace_body",
		"--dry-run",
		"--apply",
		"--allow-complex-replace",
		"complex blocks",
		"does not change document permissions",
		"does not move the document",
	} {
		if !strings.Contains(runbook, expected) {
			t.Fatalf("docs/docs-update.md missing %q:\n%s", expected, runbook)
		}
	}
}

func TestSheetsRoutingAndUpdateBoundaryAreDocumented(t *testing.T) {
	for _, relative := range []string{"README.md", "README.en.md", "docs/agent-routing.md", "docs/go-python-parity.md"} {
		text := readRepoFile(t, relative)
		for _, expected := range []string{"ixf sheets read", "ixf sheets update", "sheets update --apply"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("%s missing sheet command boundary %q:\n%s", relative, expected, text)
			}
		}
	}

	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		routingPath := filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md"))
		routing := readRepoFile(t, routingPath)
		for _, expected := range []string{"Classify the request as docs, sheets, bitable, OKR, or messenger", "direct sheets link reads", "ixf sheets update --dry-run"} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s missing sheet routing guidance %q:\n%s", routingPath, expected, routing)
			}
		}

		readerPath := filepath.ToSlash(filepath.Join(skillRoot, "ixf-docs-reader", "SKILL.md"))
		reader := readRepoFile(t, readerPath)
		for _, expected := range []string{"direct sheets link", "ixf sheets read"} {
			if !strings.Contains(reader, expected) {
				t.Fatalf("%s missing sheet read guidance %q:\n%s", readerPath, expected, reader)
			}
		}

		writerPath := filepath.ToSlash(filepath.Join(skillRoot, "ixf-docs-writer", "SKILL.md"))
		writer := readRepoFile(t, writerPath)
		for _, expected := range []string{"does not edit embedded or direct sheet cell data", "ixf sheets update --dry-run"} {
			if !strings.Contains(writer, expected) {
				t.Fatalf("%s missing sheet write boundary %q:\n%s", writerPath, expected, writer)
			}
		}
	}
}

func TestBitableRoutingAndAttachBoundaryAreDocumented(t *testing.T) {
	for _, relative := range []string{"README.md", "README.en.md", "docs/agent-routing.md", "docs/go-python-parity.md"} {
		text := readRepoFile(t, relative)
		for _, expected := range []string{"ixf bitable inspect", "ixf bitable record create", "ixf bitable attach", "record create --apply", "attach --apply"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("%s missing bitable command boundary %q:\n%s", relative, expected, text)
			}
		}
		for _, forbidden := range []string{"attach --apply` 仍失败关闭", "until the existing-record update API contract is captured"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains stale bitable attach apply guidance %q:\n%s", relative, forbidden, text)
			}
		}
	}

	for _, skillRoot := range []string{"skills", "plugins/codex/ixf-toolbox/skills", "plugins/claude/ixf-toolbox/skills"} {
		routingPath := filepath.ToSlash(filepath.Join(skillRoot, "using-ixf-toolbox", "SKILL.md"))
		routing := readRepoFile(t, routingPath)
		for _, expected := range []string{"bitable record or attachment/image upload requests", "ixf bitable record create --dry-run", "ixf bitable attach --dry-run", "do not route those requests through docs or sheets"} {
			if !strings.Contains(routing, expected) {
				t.Fatalf("%s missing bitable routing guidance %q:\n%s", routingPath, expected, routing)
			}
		}
	}
}

var currentGuidanceFiles = []string{
	"README.md",
	"README.en.md",
	"AGENTS.md",
	"SECURITY.md",
	"docs/agent-routing.md",
	"docs/go-python-parity.md",
	"docs/python-removal-readiness.md",
	"docs/python-api-sunset.md",
	"docs/release.md",
	"docs/supported-platforms.md",
}

var removedCommands = []string{
	"ixf setup skills",
	"ixf setup deps",
	"ixf update skills",
}

func TestCurrentGuidanceUsesNativePluginsAndCanonicalSkillPaths(t *testing.T) {
	for _, relative := range currentGuidanceFiles {
		content := readRepoFile(t, relative)
		for _, forbidden := range append([]string{
			"skills/codex",
			"skills/claude-code",
			"skills/*/*/SKILL.md",
		}, removedCommands...) {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s contains removed current guidance %q:\n%s", relative, forbidden, content)
			}
		}
	}

	for _, relative := range []string{"AGENTS.md", "docs/agent-routing.md", "docs/go-python-parity.md"} {
		content := readRepoFile(t, relative)
		if !strings.Contains(content, "skills/*/SKILL.md") {
			t.Fatalf("%s missing canonical skill source path:\n%s", relative, content)
		}
	}
}

func TestReadmeNativePluginInstallationAndDependencyLifecycle(t *testing.T) {
	common := []string{
		"codex plugin marketplace add serialq7ic4/ixf-toolbox",
		"codex plugin add ixf-toolbox@ixf-toolbox",
		"claude plugin marketplace add serialq7ic4/ixf-toolbox",
		"claude plugin install ixf-toolbox@ixf-toolbox --scope user --yes",
		"ixf doctor --json",
		"ixf deps install --dry-run --json",
		"ixf deps install --apply --json",
		"~/.codex/skills/ixf-*",
		"~/.claude/skills/ixf-*",
	}
	for _, tc := range []struct {
		path     string
		expected []string
	}{
		{path: "README.md", expected: []string{"首次使用", "明确确认", "不会修改 `PATH`", "不会自动删除"}},
		{path: "README.en.md", expected: []string{"first use", "explicit confirmation", "does not modify `PATH`", "not deleted automatically"}},
	} {
		content := readRepoFile(t, tc.path)
		for _, expected := range append(common, tc.expected...) {
			if !strings.Contains(content, expected) {
				t.Fatalf("%s missing native plugin lifecycle text %q:\n%s", tc.path, expected, content)
			}
		}
	}
}

func TestMigrationDocumentMarksRemovedCommandsAndManualLegacyCleanup(t *testing.T) {
	content := readRepoFile(t, "docs/migration-from-legacy.md")
	for _, expected := range []string{"Removed commands", "native plugin", "do not delete automatically"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("migration document missing %q:\n%s", expected, content)
		}
	}

	removedSection := extractMarkdownSection(content, "Removed commands")
	if removedSection == "" {
		t.Fatal("migration document has no Removed commands section")
	}
	outsideRemovedSection := strings.Replace(content, removedSection, "", 1)
	for _, command := range removedCommands {
		if !strings.Contains(removedSection, command) {
			t.Fatalf("Removed commands section missing %q:\n%s", command, removedSection)
		}
		if strings.Contains(outsideRemovedSection, command) {
			t.Fatalf("migration document uses removed command outside Removed commands section %q:\n%s", command, content)
		}
	}
}

func TestReleaseDocumentationUsesEmbeddedVersionAndNativePluginSmoke(t *testing.T) {
	content := readRepoFile(t, "docs/release.md")
	for _, forbidden := range append([]string{"-X main.version"}, removedCommands...) {
		if strings.Contains(content, forbidden) {
			t.Fatalf("release documentation contains removed instruction %q:\n%s", forbidden, content)
		}
	}
	for _, expected := range []string{"VERSION", "pluginpack --check", "smoke-native-plugins.sh", "ixf deps install --dry-run --json"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("release documentation missing %q:\n%s", expected, content)
		}
	}
}

func skillNamesForContract() []string {
	return canonicalSkillNames
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

func TestCanonicalSkillsHaveGeneratedHostCopies(t *testing.T) {
	for _, oldRoot := range []string{"skills/codex", "skills/claude-code"} {
		if _, err := os.Stat(filepath.Join(repoRoot(t), filepath.FromSlash(oldRoot))); !os.IsNotExist(err) {
			t.Fatalf("%s must not exist, stat error = %v", oldRoot, err)
		}
	}

	for _, name := range canonicalSkillNames {
		canonicalPath := filepath.Join(repoRoot(t), "skills", name, "SKILL.md")
		canonical, err := os.ReadFile(canonicalPath)
		if err != nil {
			t.Fatalf("read canonical skill %s: %v", name, err)
		}
		for _, generatedRoot := range []string{
			filepath.Join(repoRoot(t), "plugins/codex/ixf-toolbox/skills"),
			filepath.Join(repoRoot(t), "plugins/claude/ixf-toolbox/skills"),
		} {
			generatedPath := filepath.Join(generatedRoot, name, "SKILL.md")
			generated, err := os.ReadFile(generatedPath)
			if err != nil {
				t.Fatalf("read generated skill %s: %v", generatedPath, err)
			}
			if !bytes.Equal(canonical, generated) {
				t.Fatalf("%s differs from %s", generatedPath, canonicalPath)
			}
		}
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

func extractMarkdownSection(markdown string, heading string) string {
	header := "## " + heading
	start := strings.Index(markdown, header)
	if start < 0 {
		return ""
	}
	body := markdown[start:]
	next := strings.Index(body[len(header):], "\n## ")
	if next >= 0 {
		body = body[:len(header)+next]
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
