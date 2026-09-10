# Native Plugin Installation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `ixf-toolbox` 3.27.0 with native Codex and Claude Code plugins, confirmed first-use Go runtime bootstrap, read-only installation diagnostics, and an explicit `ixf deps install` repair command.

**Architecture:** Keep the seven skills in one canonical `skills/` tree. A Go packaging tool deterministically generates separate Codex and Claude plugin packages so Claude alone receives the SessionStart hook, while both plugins share the same skills and bootstrap metadata. The Go `ixf` binary remains the only business runtime; `doctor` only diagnoses, `deps install --apply` performs optional dependency changes, and native host plugin commands own skill installation.

**Tech Stack:** Go 1.24 standard library, POSIX shell, Windows PowerShell, Codex plugin manifests, Claude Code plugin manifests, GitHub Actions.

## Global Constraints

- Release version is `3.27.0`; this is a feature release, not a patch release.
- The repository and runtime remain Go-only. Do not add Python source, Python fallback, `ixfdoc`, or `ixfwrite` calls.
- Native Codex and Claude Code plugins are the only supported new skill installation path.
- Remove `ixf setup skills`, `ixf update skills`, and the entire `ixf setup` command family.
- `ixf doctor --json` is read-only: it may inspect files, run read-only host CLI queries, and perform the existing update reachability check, but it must not install, download, delete, or modify anything.
- Optional dependency mutation is available only through `ixf deps install --apply`; dry-run is the default and `--dry-run` remains explicit and supported.
- Runtime bootstrap must require `--apply`, must verify the release checksum, must install only under the current user's directory, and must not change `PATH`.
- Runtime bootstrap and `deps install` must never install Chrome, alter Messenger profiles, change LarkShell login state, or configure a proxy.
- Existing raw skills under `~/.codex/skills` and `~/.claude/skills` are diagnosed as legacy but are never deleted automatically.
- GitHub Release continues to publish only Go binaries and the checksum file; plugin packages are consumed from the repository marketplaces.
- All GitHub network operations must use `http://127.0.0.1:7890` through per-command environment variables, not global git configuration.

---

## File Map

**Canonical source and packaging**

- Create: `plugin-src/metadata.json`
- Create: `plugin-src/scripts/bootstrap-runtime.sh`
- Create: `plugin-src/scripts/bootstrap-runtime.ps1`
- Create: `plugin-src/claude/hooks/hooks.json`
- Create: `plugin-src/claude/hooks/run-hook.cmd`
- Create: `plugin-src/claude/hooks/session-start`
- Create: `internal/pluginpack/pluginpack.go`
- Create: `internal/pluginpack/pluginpack_test.go`
- Create: `cmd/pluginpack/main.go`
- Create: `scripts/build-plugin-packages.sh`
- Create generated: `.agents/plugins/marketplace.json`
- Create generated: `.claude-plugin/marketplace.json`
- Create generated: `plugins/codex/ixf-toolbox/**`
- Create generated: `plugins/claude/ixf-toolbox/**`
- Move: `skills/codex/*/SKILL.md` to `skills/*/SKILL.md`
- Delete: `skills/claude-code/`

**Runtime modules and CLI**

- Create: `internal/agentinstall/diagnostics.go`
- Create: `internal/agentinstall/diagnostics_test.go`
- Create: `internal/dependencies/dependencies.go`
- Create: `internal/dependencies/dependencies_test.go`
- Create: `version_embed.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Delete: `skills_embed.go`

## Test Helper Contracts

The plan uses only the following helpers. Helpers already present in the repository are reused
without renaming in the package where they already exist: `runCLITest`, `decodeCLIJSON`,
`writeCLICookieFixture`, `mustReadFile`, `shellQuote`, `readRepoFile`, and `repoRoot`. New helpers
are defined in the test package where they are used; no test step may call an undeclared helper.

### `internal/pluginpack` helpers

Add these helpers to `internal/pluginpack/pluginpack_test.go` before the generator tests:

```go
func newPluginFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadata := `{"name":"ixf-toolbox","description":"fixture","author":{"name":"fixture"},"homepage":"https://example.test/ixf-toolbox","repository":"https://example.test/ixf-toolbox","license":"Apache-2.0","keywords":["ixf"],"interface":{"displayName":"i讯飞 Toolbox","shortDescription":"Authorized i讯飞 workflows","longDescription":"Use authorized i讯飞 and LarkShell workflows through the Go ixf runtime.","developerName":"fixture","category":"Productivity","capabilities":["Read","Write"],"defaultPrompt":["Read an i讯飞 document","Plan an approved document update"]}}`
	if err := os.MkdirAll(filepath.Join(root, "plugin-src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin-src", "metadata.json"), []byte(metadata+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"using-ixf-toolbox",
		"ixf-docs-reader",
		"ixf-docs-writer",
		"ixf-okr-reader",
		"ixf-okr-writer",
		"ixf-messenger-reader",
		"ixf-messenger-writer",
	} {
		dir := filepath.Join(root, "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\nfixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return payload
}

func assertSameFile(t *testing.T, wantPath, gotPath string) {
	t.Helper()
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("%s and %s differ", wantPath, gotPath)
	}
}
```

`pluginpack_test.go` must import `bytes`, `encoding/json`, `errors`, `os`, `path/filepath`, `strings`, and
`testing` for these helpers and the tests below.

`bootstrap_test.go` is created in Task 2. It reuses `newPluginFixture`, and defines the following
helpers there: `generateRepositoryFixture(t) string`, `bootstrapFixtureServer(t, version string,
asset, checksumAsset []byte, requests *atomic.Int32) *httptest.Server`,
`bootstrapEnv(t, overrides map[string]string) []string`,
`runBootstrapCommand(t, root string, overrides map[string]string, args ...string) ([]byte, error)`,
`runBootstrap(t, root string, overrides map[string]string, args ...string) bootstrapResult`,
`readBootstrapFile(t, path string) []byte`, and `runSessionStart(t, pluginRoot, input string) []byte`.
Their contracts are shown in Task 2; the fixture copies the repository's `plugin-src` tree into a
temp root and then calls `Generate(Options{Root: root})`, so the test exercises the real templates
rather than a second test implementation of them. `bootstrapEnv` must merge `os.Environ()` by key,
replace overridden values, sort the resulting keys, and never append duplicate `HOME`,
`LOCALAPPDATA`, proxy, or test-control entries. The bootstrap tests use `readBootstrapFile`, not a
helper from `cmd/ixf`.

`diagnostics_test.go` uses `package agentinstall_test` and defines
`fixtureOptions(t, codexJSON, claudeJSON string) agentinstall.Options` with an injected
`agentinstall.CommandRunner`, plus `snapshotTree(t, root string) map[string]treeEntry` and
`cmpTree(before, after map[string]treeEntry) string`. The tree helpers walk regular files only and
compare relative path, file mode, and bytes. The command fixture returns the supplied JSON for
`codex plugin list --json` and `claude plugin list --json`; all other commands return an error.
Production types and functions used by this external test package must therefore be exported from
`internal/agentinstall`.

`cmd/ixf/main_test.go` must define its own `homeTreeEntry`, `snapshotHomeTree(t, root string)`,
and `compareHomeTree(before, after map[string]homeTreeEntry) string`; it must not reuse the
`internal/agentinstall` test helpers across Go packages. These helpers use the same regular-file,
mode, and byte comparison rules for the doctor side-effect test.

**Contracts, documentation, and release**

- Modify: `repository_contract_test.go`
- Modify: `scripts/smoke-go-binary.sh`
- Create: `scripts/smoke-native-plugins.sh`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `AGENTS.md`
- Modify: `SECURITY.md`
- Modify: `docs/agent-routing.md`
- Modify: `docs/go-python-parity.md`
- Modify: `docs/python-removal-readiness.md`
- Modify: `docs/python-api-sunset.md`
- Modify: `docs/migration-from-legacy.md`
- Modify: `docs/release.md`
- Modify: `docs/supported-platforms.md`
- Modify: `CHANGELOG.md`
- Modify: `VERSION`

---

### Task 1: Canonical Skills And Host-Specific Plugin Packages

**Files:**
- Create: `plugin-src/metadata.json`
- Create: `internal/pluginpack/pluginpack.go`
- Create: `internal/pluginpack/pluginpack_test.go`
- Create: `cmd/pluginpack/main.go`
- Create: `scripts/build-plugin-packages.sh`
- Create generated: `.agents/plugins/marketplace.json`
- Create generated: `.claude-plugin/marketplace.json`
- Create generated: `plugins/codex/ixf-toolbox/**`
- Create generated: `plugins/claude/ixf-toolbox/**`
- Move: `skills/codex/*/SKILL.md` to `skills/*/SKILL.md`
- Delete: `skills/claude-code/`
- Modify: `repository_contract_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Create: `version_embed.go`
- Delete: `skills_embed.go`

**Interfaces:**
- Consumes: root `VERSION`, `plugin-src/metadata.json`, and canonical `skills/<name>/SKILL.md` files.
- Produces: `pluginpack.Generate(pluginpack.Options{Root: string, Check: bool}) error`.
- Produces: `go run ./cmd/pluginpack` to refresh generated packages and `go run ./cmd/pluginpack --check` to reject drift.
- Preserves: `ixftoolbox.DefaultVersion`, now embedded by `version_embed.go` without `SkillFS`.

- [ ] **Step 1: Add failing generator and repository contract tests**

Add tests that require one canonical skill tree, deterministic generated packages, matching versions, and host-specific manifest boundaries. The core test contract is:

```go
func TestGenerateCreatesHostSpecificPackages(t *testing.T) {
	root := newPluginFixture(t, "1.2.3")
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	codex := readJSONMap(t, filepath.Join(root, "plugins/codex/ixf-toolbox/.codex-plugin/plugin.json"))
	if codex["name"] != "ixf-toolbox" || codex["version"] != "1.2.3" || codex["skills"] != "./skills/" {
		t.Fatalf("codex manifest = %#v", codex)
	}
	assertCodexInterface(t, codex)
	if _, exists := codex["hooks"]; exists {
		t.Fatalf("codex manifest must not declare hooks: %#v", codex)
	}
	if _, err := os.Stat(filepath.Join(root, "plugins/codex/ixf-toolbox/hooks")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("codex package unexpectedly contains hooks: %v", err)
	}

	claude := readJSONMap(t, filepath.Join(root, "plugins/claude/ixf-toolbox/.claude-plugin/plugin.json"))
	if claude["name"] != "ixf-toolbox" || claude["version"] != "1.2.3" {
		t.Fatalf("claude manifest = %#v", claude)
	}
	for _, key := range []string{"hooks", "skills", "interface"} {
		if _, exists := claude[key]; exists {
			t.Fatalf("Claude manifest must rely on root discovery and omit %q: %#v", key, claude)
		}
	}
	assertSameFile(t,
		filepath.Join(root, "skills/using-ixf-toolbox/SKILL.md"),
		filepath.Join(root, "plugins/codex/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md"),
	)
	assertSameFile(t,
		filepath.Join(root, "skills/using-ixf-toolbox/SKILL.md"),
		filepath.Join(root, "plugins/claude/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md"),
	)
}

func TestGeneratedMarketplacesPointAtHostPackages(t *testing.T) {
	root := newPluginFixture(t, "1.2.3")
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	codexMarketplace := readJSONMap(t, filepath.Join(root, ".agents/plugins/marketplace.json"))
	claudeMarketplace := readJSONMap(t, filepath.Join(root, ".claude-plugin/marketplace.json"))
	if codexMarketplace["name"] != "ixf-toolbox" || claudeMarketplace["name"] != "ixf-toolbox" {
		t.Fatalf("marketplace names = %#v, %#v", codexMarketplace["name"], claudeMarketplace["name"])
	}
	codexEntry := codexMarketplace["plugins"].([]any)[0].(map[string]any)
	if codexEntry["source"].(map[string]any)["path"] != "./plugins/codex/ixf-toolbox" {
		t.Fatalf("Codex source = %#v", codexEntry["source"])
	}
	if codexEntry["policy"].(map[string]any)["installation"] != "AVAILABLE" ||
		codexEntry["policy"].(map[string]any)["authentication"] != "ON_USE" {
		t.Fatalf("Codex policy = %#v", codexEntry["policy"])
	}
	if codexEntry["category"] != "Productivity" {
		t.Fatalf("Codex marketplace metadata = %#v", codexEntry)
	}
	claudeEntry := claudeMarketplace["plugins"].([]any)[0].(map[string]any)
	if claudeEntry["source"] != "./plugins/claude/ixf-toolbox" {
		t.Fatalf("Claude source = %#v", claudeEntry["source"])
	}
	if claudeEntry["category"] != "Productivity" {
		t.Fatalf("Claude marketplace metadata = %#v", claudeEntry)
	}
}

func TestCheckRejectsGeneratedPackageDrift(t *testing.T) {
	root := newPluginFixture(t, "1.2.3")
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "plugins/codex/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md")
	if err := os.WriteFile(path, []byte("drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Generate(Options{Root: root, Check: true}); err == nil || !strings.Contains(err.Error(), "generated plugin packages are stale") {
		t.Fatalf("check error = %v", err)
	}
}

func assertCodexInterface(t *testing.T, manifest map[string]any) {
	t.Helper()
	interfaceValue, ok := manifest["interface"].(map[string]any)
	if !ok {
		t.Fatalf("Codex manifest has no interface object: %#v", manifest)
	}
	for _, key := range []string{
		"displayName", "shortDescription", "longDescription", "developerName",
		"category", "capabilities", "defaultPrompt",
	} {
		if _, ok := interfaceValue[key]; !ok {
			t.Fatalf("Codex interface missing %q: %#v", key, interfaceValue)
		}
	}
}
```

Update repository contracts to require exactly these seven canonical skill directories and reject `skills/codex` or `skills/claude-code`:

```go
var canonicalSkillNames = []string{
	"using-ixf-toolbox",
	"ixf-docs-reader",
	"ixf-docs-writer",
	"ixf-okr-reader",
	"ixf-okr-writer",
	"ixf-messenger-reader",
	"ixf-messenger-writer",
}
```

In the same task, migrate every existing repository-contract test that currently loops over
`skills/codex` or `skills/claude-code` to use the canonical path
`skills/<skill-name>/SKILL.md`. Add two explicit generated-package loops for
`plugins/codex/ixf-toolbox/skills/<skill-name>/SKILL.md` and
`plugins/claude/ixf-toolbox/skills/<skill-name>/SKILL.md`; each must compare bytes with the
canonical file. This includes `TestAgentRoutingContractIsAuthoritativeAndNatural`,
`TestLocalMarkdownDoesNotDefaultToIxfDocsReader`, `TestIxfSkillsRejectPythonAndLegacyFallbacks`,
`TestMessengerSkillsAreRoutedAndDocumentDryRunSafety`, `TestDocsWriterSkillDoesNotOverclaimExistingDocumentUpdate`,
`TestDocsWriterSkillRoutesLocalizedInsertToPatch`, `TestSheetsRoutingAndUpdateBoundaryAreDocumented`,
and `TestBitableRoutingAndAttachBoundaryAreDocumented`. No repository test may retain an old
runtime-specific skill path as a read target; a separate negative assertion may still verify that
those directories do not exist.

- [ ] **Step 2: Run the focused tests and confirm they fail**

Run:

```bash
go test ./internal/pluginpack ./... -run 'TestGenerateCreatesHostSpecificPackages|TestGeneratedMarketplaces|TestCheckRejectsGeneratedPackageDrift|TestCanonicalSkills'
```

Expected: FAIL because `internal/pluginpack`, canonical `skills/`, and generated plugin packages do not exist yet.

- [ ] **Step 3: Implement the deterministic Go package generator**

Implement this public contract:

```go
package pluginpack

type Options struct {
	Root  string
	Check bool
}

func Generate(options Options) error
```

`Generate` must:

- Read and validate strict `X.Y.Z` from `VERSION`.
- Decode `plugin-src/metadata.json` with `encoding/json`.
- Require all seven canonical skills and copy only regular files; reject symlinks.
- Generate valid JSON with `json.MarshalIndent`, a trailing newline, and stable field content.
- Generate separate host manifests from the shared metadata rather than copying one JSON object
  into both hosts. The Codex manifest must include the complete `interface` object with
  `displayName`, `shortDescription`, `longDescription`, `developerName`, `category`,
  `capabilities`, and `defaultPrompt`; it explicitly sets `skills: "./skills/"` and never contains
  `hooks`. The Claude manifest must contain only fields accepted by Claude Code's plugin manifest
  schema, relies on conventional root `skills/` and `hooks/` discovery, and must not copy Codex-only
  `interface` or `skills` fields unless the installed validator explicitly accepts them and a test
  pins that contract.
- Generate `.agents/plugins/marketplace.json` with a Codex local source path
  `./plugins/codex/ixf-toolbox`, `policy.installation: "AVAILABLE"`,
  `policy.authentication: "ON_USE"`, and `category: "Productivity"`.
- Generate `.claude-plugin/marketplace.json` with Claude source `./plugins/claude/ixf-toolbox`,
  a `$schema` of `https://anthropic.com/claude-code/marketplace.schema.json`, an owner name, and
  the plugin metadata required by Claude's marketplace validator. Keep this marketplace schema
  distinct from the Codex marketplace policy schema; validate both with their respective parsers.
- Build expected output in a temporary directory, then replace only the four known generated roots in write mode.
- Compare relative file names, bytes, and executable mode in check mode; return a sorted drift list without modifying files.

Use this metadata shape:

```json
{
  "name": "ixf-toolbox",
  "description": "Use authorized i讯飞 and LarkShell workflows through the Go ixf runtime.",
  "author": {"name": "serialq7ic4"},
  "homepage": "https://github.com/serialq7ic4/ixf-toolbox",
  "repository": "https://github.com/serialq7ic4/ixf-toolbox",
  "license": "Apache-2.0",
  "keywords": ["ixf", "i讯飞", "LarkShell", "docx", "okr", "bitable", "messenger"],
  "interface": {
    "displayName": "i讯飞 Toolbox",
    "shortDescription": "Authorized i讯飞 workflows",
    "longDescription": "Use authorized i讯飞 and LarkShell workflows through the Go ixf runtime.",
    "developerName": "serialq7ic4",
    "category": "Productivity",
    "capabilities": ["Read", "Write"],
    "defaultPrompt": ["Read an i讯飞 document", "Plan an approved document update"]
  }
}
```

The shell wrapper must contain only:

```sh
#!/bin/sh
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"
exec go run ./cmd/pluginpack "$@"
```

- [ ] **Step 4: Move skills and remove binary-owned skill installation**

Move the Codex copies to `skills/<name>/SKILL.md`, verify each old Claude file is byte-identical
before deleting `skills/claude-code`, and remove the binary-owned skill installer completely.
Delete these symbols from `cmd/ixf/main.go` and remove every test that calls them:

```text
runtimeTarget
skillResult
runSetupSkills
runUpdateSkills
installSkills
detectRuntimeTargets
normalizeRuntimes
skillNames
skillsStatus
```

Keep `runSetupDeps` and `ixf setup deps` temporarily so dependency command migration remains
isolated in Task 4, but remove `runSetupSkills` and `runUpdateSkills`; `ixf setup skills` and
`ixf update skills` must already fail closed. Remove the `skills` field and the skill-dependent
health calculation from `collectDiagnostics`; until Task 3 adds `agentRouting.installation`,
`doctor.ok` must be based on cookie readiness only. Update `formatDiagnostics` and its fixtures so
they no longer print a `skill <runtime>` section. Delete the old
`TestNormalizeRuntimesSupportsAutoAliasesAndValidation` and
`TestInstallSkillsWritesEmbeddedCodexSkillsAndPreservesExistingWithoutForce` tests; replace their
skill-content assertions with canonical/generated package parity assertions in
`repository_contract_test.go`. Create `version_embed.go`, delete `skills_embed.go`, and update
`repository_contract_test.go` to read `version_embed.go` for the version-source assertion and to
assert that `skills_embed.go` is absent. Its complete contents are:

```go
package ixftoolbox

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var rawVersion string

var DefaultVersion = strings.TrimSpace(rawVersion)
```

- [ ] **Step 5: Generate packages and make focused tests pass**

Run:

```bash
go run ./cmd/pluginpack
go run ./cmd/pluginpack --check
go test ./internal/pluginpack ./cmd/ixf .
```

Expected: PASS. Also confirm `git status --short` shows generated plugin packages and no
`skills/codex` or `skills/claude-code` paths, and run `rg` over `cmd/ixf` and its tests to confirm
that no removed installer symbol remains.

- [ ] **Step 6: Commit the canonical plugin package foundation**

```bash
git add plugin-src internal/pluginpack cmd/pluginpack scripts/build-plugin-packages.sh \
  .agents/plugins/marketplace.json .claude-plugin/marketplace.json plugins skills \
  version_embed.go cmd/ixf/main.go cmd/ixf/main_test.go repository_contract_test.go
git commit -m "feat: add native plugin packages"
```

---

### Task 2: Confirmed Runtime Bootstrap And Claude Discovery Hook

**Files:**
- Create: `plugin-src/scripts/bootstrap-runtime.sh`
- Create: `plugin-src/scripts/bootstrap-runtime.ps1`
- Create: `plugin-src/claude/hooks/hooks.json`
- Create: `plugin-src/claude/hooks/session-start`
- Create: `internal/pluginpack/bootstrap_test.go`
- Modify: `internal/pluginpack/pluginpack.go`
- Modify generated: `plugins/codex/ixf-toolbox/**`
- Modify generated: `plugins/claude/ixf-toolbox/**`

**Interfaces:**
- Produces: `bootstrap-runtime.sh [--dry-run|--apply] [--install-dir DIR]`.
- Produces: `bootstrap-runtime.ps1 [-DryRun|-Apply] [-InstallDir DIR]`.
- Produces: generated `runtime.json` with `schemaVersion`, `repository`, `releaseVersion`, `minimumVersion`, supported artifacts, and user install paths.
- Produces: Claude `SessionStart` output under `hookSpecificOutput.additionalContext`.

- [ ] **Step 1: Add failing bootstrap and hook tests**

Cover dry-run, explicit apply, checksum mismatch, install location, no PATH mutation, and hook output. The Unix integration test must use an `httptest.Server` and permit its HTTP URL only when `IXF_BOOTSTRAP_TESTING=1`:

Add these test-only types and helpers to `internal/pluginpack/bootstrap_test.go` before the tests:

```go
type bootstrapResult struct {
	OK          bool   `json:"ok"`
	DryRun      bool   `json:"dryRun"`
	Apply       bool   `json:"apply"`
	Version     string `json:"version"`
	Asset       string `json:"asset"`
	ReleaseHost string `json:"releaseHost"`
	TargetPath  string `json:"targetPath"`
}

func generateRepositoryFixture(t *testing.T) string {
	t.Helper()
	root := newPluginFixture(t, "1.2.3")
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	copyRegularTree(t, filepath.Join(repositoryRoot, "plugin-src"), filepath.Join(root, "plugin-src"))
	if err := Generate(Options{Root: root}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	return root
}

func copyRegularTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("fixture source %s is a symlink", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture source %s is not a regular file", path)
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
	if err != nil {
		t.Fatal(err)
	}
}

func bootstrapFixtureServer(t *testing.T, version string, asset, checksumAsset []byte, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	assetName := fmt.Sprintf("ixf_%s_%s_%s", version, runtime.GOOS, runtime.GOARCH)
	checksumName := fmt.Sprintf("ixf_%s_checksums.txt", version)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	digest := sha256.Sum256(checksumAsset)
	checksum := fmt.Sprintf("%x  %s\n", digest, assetName)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+assetName):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(asset)
		case strings.HasSuffix(r.URL.Path, "/"+checksumName):
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, checksum)
		default:
			http.NotFound(w, r)
		}
	}))
}

func bootstrapEnv(t *testing.T, overrides map[string]string) []string {
	t.Helper()
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	if _, ok := overrides["HOME"]; !ok {
		values["HOME"] = t.TempDir()
	}
	if _, ok := overrides["LOCALAPPDATA"]; !ok {
		values["LOCALAPPDATA"] = values["HOME"]
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func runBootstrapCommand(t *testing.T, root string, overrides map[string]string, args ...string) ([]byte, error) {
	t.Helper()
	pluginRoot := filepath.Join(root, "plugins", "codex", "ixf-toolbox")
	command := exec.Command(filepath.Join(pluginRoot, "scripts", "bootstrap-runtime.sh"), args...)
	command.Env = bootstrapEnv(t, overrides)
	return command.CombinedOutput()
}

func runBootstrap(t *testing.T, root string, overrides map[string]string, args ...string) bootstrapResult {
	t.Helper()
	output, err := runBootstrapCommand(t, root, overrides, args...)
	if err != nil {
		t.Fatalf("bootstrap %v: %v\n%s", args, err, output)
	}
	var result bootstrapResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode bootstrap output: %v\n%s", err, output)
	}
	return result
}

func readBootstrapFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func runPowerShellBootstrapCommand(t *testing.T, root string, overrides map[string]string, args ...string) ([]byte, error) {
	t.Helper()
	powershell, err := exec.LookPath("pwsh")
	if err != nil {
		powershell, err = exec.LookPath("powershell")
		if err != nil {
			t.Skip("PowerShell is not installed")
		}
	}
	script := filepath.Join(root, "plugins", "codex", "ixf-toolbox", "scripts", "bootstrap-runtime.ps1")
	commandArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(powershell, commandArgs...)
	command.Env = bootstrapEnv(t, overrides)
	return command.CombinedOutput()
}

func runSessionStart(t *testing.T, pluginRoot, input string) []byte {
	t.Helper()
	dispatcher := filepath.Join(pluginRoot, "hooks", "run-hook.cmd")
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		cmdExe, err := exec.LookPath("cmd.exe")
		if err != nil {
			t.Fatal("cmd.exe is required on Windows")
		}
		command = exec.Command(cmdExe, "/d", "/c", dispatcher, "session-start")
	} else {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is required to execute the polyglot hook dispatcher")
		}
		command = exec.Command(bash, dispatcher, "session-start")
	}
	command.Stdin = strings.NewReader(input)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("session-start: %v\n%s", err, output)
	}
	if runtime.GOOS == "windows" && len(bytes.TrimSpace(output)) == 0 {
		t.Skip("Windows hook dispatcher found no supported Bash executable")
	}
	return output
}
```

The helper imports are `bytes`, `crypto/sha256`, `encoding/json`, `fmt`, `io`, `net/http`,
`net/http/httptest`, `os`, `os/exec`, `io/fs`, `path/filepath`, `runtime`, `sort`, `strings`,
`sync/atomic`, and `testing`. `copyRegularTree` rejects symlinks and preserves executable bits,
which lets package generation tests catch script mode drift. `bootstrapEnv` replaces environment
entries by key rather than appending duplicate `HOME`, `LOCALAPPDATA`, proxy, or test variables.

```go
func TestUnixBootstrapRequiresExplicitApplyAndVerifiesChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, asset, &requests)
	t.Cleanup(server.Close)
	home := t.TempDir()
	target := filepath.Join(home, "bin", "ixf")
	pathBefore := os.Getenv("PATH")

	dryRun := runBootstrap(t, root, map[string]string{
		"HOME":                           home,
		"PATH":                           pathBefore,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}, "--dry-run", "--install-dir", filepath.Dir(target))
	if !dryRun.OK || dryRun.Apply || !dryRun.DryRun || dryRun.Version != "1.2.3" || dryRun.ReleaseHost == "" {
		t.Fatalf("dry-run payload = %#v", dryRun)
	}
	if requests.Load() != 0 {
		t.Fatalf("dry-run contacted release server %d times", requests.Load())
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote target: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run created install directory: %v", err)
	}

	apply := runBootstrap(t, root, map[string]string{
		"HOME":                           home,
		"PATH":                           pathBefore,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}, "--apply", "--install-dir", filepath.Dir(target))
	if !apply.OK || !apply.Apply || apply.DryRun || apply.TargetPath != target || requests.Load() != 2 {
		t.Fatalf("apply payload = %#v, requests=%d", apply, requests.Load())
	}
	if got := readBootstrapFile(t, target); !bytes.Equal(got, asset) {
		t.Fatalf("installed bytes = %q", got)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary is not executable: info=%v err=%v", info, err)
	}
	if os.Getenv("PATH") != pathBefore {
		t.Fatalf("parent PATH changed from %q to %q", pathBefore, os.Getenv("PATH"))
	}
	for _, profile := range []string{".profile", ".bashrc", ".zprofile", ".zshrc"} {
		if _, err := os.Stat(filepath.Join(home, profile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("bootstrap modified shell profile %s: %v", profile, err)
		}
	}
}

func TestUnixBootstrapDefaultsToDryRunAndRejectsConflictingModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	home := t.TempDir()
	for _, args := range [][]string{{}, {"--dry-run", "--apply"}} {
		output, err := runBootstrapCommand(t, root, map[string]string{"HOME": home}, args...)
		if len(args) == 0 {
			if err != nil {
				t.Fatalf("bootstrap without mode failed: %v\n%s", err, output)
			}
			var result bootstrapResult
			if json.Unmarshal(output, &result) != nil || !result.OK || !result.DryRun || result.Apply {
				t.Fatalf("default mode output = %s", output)
			}
			continue
		}
		if err == nil || !strings.Contains(string(output), "mutually exclusive") {
			t.Fatalf("conflicting modes accepted: err=%v output=%s", err, output)
		}
	}
}

func TestUnixBootstrapRejectsInstallDirOutsideHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	home := t.TempDir()
	outside := t.TempDir()
	output, err := runBootstrapCommand(t, root, map[string]string{"HOME": home}, "--apply", "--install-dir", outside)
	if err == nil || !strings.Contains(string(output), "inside the current user directory") {
		t.Fatalf("outside install directory accepted: err=%v output=%s", err, output)
	}
}

func TestUnixBootstrapRefusesChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, []byte("different bytes"), &requests)
	t.Cleanup(server.Close)
	home := t.TempDir()
	targetDir := filepath.Join(home, "bin")
	output, err := runBootstrapCommand(t, root, map[string]string{
		"HOME":                           home,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}, "--apply", "--install-dir", targetDir)
	if err == nil || strings.Contains(string(output), `"ok":true`) {
		t.Fatalf("checksum mismatch unexpectedly succeeded: err=%v output=%s", err, output)
	}
	if entries, readErr := os.ReadDir(targetDir); readErr == nil && len(entries) != 0 {
		t.Fatalf("checksum mismatch wrote install directory: %v", entries)
	}
}

func TestUnixBootstrapRejectsUntrustedReleaseOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := generateRepositoryFixture(t)
	output, err := runBootstrapCommand(t, root, map[string]string{
		"HOME":                           t.TempDir(),
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": "http://127.0.0.1:1",
	}, "--dry-run")
	if err == nil || !strings.Contains(string(output), "HTTPS") {
		t.Fatalf("untrusted release override accepted: err=%v output=%s", err, output)
	}
}

func TestPowerShellBootstrapRequiresApplyAndVerifiesChecksum(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell bootstrap test")
	}
	root := generateRepositoryFixture(t)
	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, asset, &requests)
	t.Cleanup(server.Close)
	localAppData := t.TempDir()
	target := filepath.Join(localAppData, "bin", "ixf.exe")
	overrides := map[string]string{
		"LOCALAPPDATA":                   localAppData,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}
	dryOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-DryRun", "-InstallDir", filepath.Dir(target))
	if err != nil {
		t.Fatalf("PowerShell dry-run failed: %v\n%s", err, dryOutput)
	}
	var dryRun bootstrapResult
	if err := json.Unmarshal(dryOutput, &dryRun); err != nil || !dryRun.OK || !dryRun.DryRun || dryRun.Apply || requests.Load() != 0 {
		t.Fatalf("PowerShell dry-run = %s, requests=%d, err=%v", dryOutput, requests.Load(), err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PowerShell dry-run wrote target: %v", err)
	}
	applyOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-Apply", "-InstallDir", filepath.Dir(target))
	if err != nil {
		t.Fatalf("PowerShell apply failed: %v\n%s", err, applyOutput)
	}
	var apply bootstrapResult
	if err := json.Unmarshal(applyOutput, &apply); err != nil || !apply.OK || !apply.Apply || apply.TargetPath != target || requests.Load() != 2 {
		t.Fatalf("PowerShell apply = %s, requests=%d, err=%v", applyOutput, requests.Load(), err)
	}
	if got := readBootstrapFile(t, target); !bytes.Equal(got, asset) {
		t.Fatalf("PowerShell installed bytes = %q", got)
	}
}

func TestPowerShellBootstrapRejectsInvalidModesPathsAndChecksum(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell bootstrap test")
	}
	root := generateRepositoryFixture(t)
	localAppData := t.TempDir()
	for _, args := range [][]string{{"-DryRun", "-Apply"}, {"-Apply", "-InstallDir", t.TempDir()}} {
		output, err := runPowerShellBootstrapCommand(t, root, map[string]string{"LOCALAPPDATA": localAppData}, args...)
		if err == nil || (!strings.Contains(string(output), "mutually exclusive") && !strings.Contains(string(output), "inside the current user directory")) {
			t.Fatalf("invalid PowerShell invocation accepted: args=%v err=%v output=%s", args, err, output)
		}
	}

	asset := []byte("fixture ixf binary")
	var requests atomic.Int32
	server := bootstrapFixtureServer(t, "1.2.3", asset, []byte("different bytes"), &requests)
	t.Cleanup(server.Close)
	targetDir := filepath.Join(localAppData, "bin")
	overrides := map[string]string{
		"LOCALAPPDATA":                   localAppData,
		"IXF_BOOTSTRAP_TESTING":          "1",
		"IXF_BOOTSTRAP_RELEASE_BASE_URL": server.URL,
	}
	dryOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-DryRun", "-InstallDir", targetDir)
	if err != nil {
		t.Fatalf("PowerShell checksum dry-run failed: %v\n%s", err, dryOutput)
	}
	if requests.Load() != 0 {
		t.Fatalf("PowerShell dry-run contacted release server %d times", requests.Load())
	}
	applyOutput, err := runPowerShellBootstrapCommand(t, root, overrides, "-Apply", "-InstallDir", targetDir)
	if err == nil || strings.Contains(string(applyOutput), `"ok":true`) {
		t.Fatalf("PowerShell checksum mismatch unexpectedly succeeded: err=%v output=%s", err, applyOutput)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "ixf.exe")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PowerShell checksum mismatch wrote target: %v", err)
	}
}
```

The hook test must assert exactly one short routing hint and no executable behavior. Its helper must
invoke the polyglot dispatcher through `bash <dispatcher> session-start` on POSIX; directly invoking
the extensionless dispatcher from `os/exec` is invalid because it has no shebang. On Windows invoke
the checked-in dispatcher through `cmd.exe /d /c`; if it exits with empty output because no supported
Bash executable is available, skip with an explicit reason rather than treating the hook as a pass:

```go
func TestClaudeSessionStartHookOnlyInjectsRoutingHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook test; Windows host validation uses Claude's shell dispatcher")
	}
	root := generateRepositoryFixture(t)
	output := runSessionStart(t, filepath.Join(root, "plugins/claude/ixf-toolbox"), `{}`)
	var payload struct {
		Hook struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Hook.Event != "SessionStart" || payload.Hook.Context != "For i讯飞 / LarkShell links or document, sheets, bitable, OKR, and Messenger requests, consider the ixf-toolbox skills before choosing generic tools." {
		t.Fatalf("hook payload = %#v", payload)
	}
	for _, forbidden := range []string{"curl ", "ixf doctor", "cookies export", "setup"} {
		if strings.Contains(payload.Hook.Context, forbidden) {
			t.Fatalf("hook context executes work through %q", forbidden)
		}
	}
}
```

Add a second hook test that decodes `hooks/hooks.json` with `encoding/json` and asserts one
`SessionStart` registration, the `startup|clear|compact` matcher, the quoted
`${CLAUDE_PLUGIN_ROOT}` reference, and `async: false`. The test must not validate JSON with shell
`grep`. On Windows, run the same registration test and run the executable hook through the checked
in Claude shell dispatcher (or skip only the POSIX execution assertion when no supported shell is
available); do not silently mark a missing Windows hook as passing.

- [ ] **Step 2: Run tests and confirm they fail**

```bash
go test ./internal/pluginpack -run 'TestUnixBootstrap|TestPowerShellBootstrap|TestClaudeSessionStart'
```

Expected: FAIL because bootstrap scripts, runtime metadata, and Claude hook do not exist.

- [ ] **Step 3: Implement generated runtime metadata and bootstrap scripts**

Generate this shape from `VERSION`, never by parsing or replacing JSON text. In the fixture tests,
assert that both `releaseVersion` and `minimumVersion` equal the fixture `VERSION`; the final
repository output uses `3.27.0`:

```json
{
  "schemaVersion": 1,
  "repository": "serialq7ic4/ixf-toolbox",
  "releaseVersion": "3.27.0",
  "minimumVersion": "3.27.0",
  "installPaths": {
    "unix": "~/.local/share/ixf-toolbox/bin/ixf",
    "windows": "%LOCALAPPDATA%\\ixf-toolbox\\bin\\ixf.exe"
  },
  "artifacts": [
    "darwin/amd64",
    "darwin/arm64",
    "linux/amd64",
    "linux/arm64",
    "windows/amd64"
  ]
}
```

Generate version and repository constants into both scripts so neither script needs `jq`, Node, Python, or ad hoc JSON parsing. Both scripts must:

- Default to dry-run and reject simultaneous dry-run/apply switches.
- Resolve the platform only from the supported matrix.
- Build `https://github.com/serialq7ic4/ixf-toolbox/releases/download/vX.Y.Z/...` URLs.
- Inherit standard proxy environment variables without writing proxy configuration.
- Download the platform artifact and `ixf_X.Y.Z_checksums.txt` to a temporary directory.
- Match the exact artifact filename in the checksum file and verify SHA-256.
- Create only the selected user install directory.
- Atomically replace the user binary and set executable mode on Unix.
- Emit secret-safe JSON with `ok`, `dryRun`, `apply`, `version`, `asset`, `releaseHost`, and
  `targetPath`; never include cookies, authorization headers, or the full test URL in output.
- Reject non-HTTPS release URLs unless both test environment variables are set.
- Require an explicit install directory to resolve inside `HOME` on Unix or `LOCALAPPDATA` on
  Windows. Reject absolute paths outside that root and traversal/symlink escapes before any network
  request or directory creation. The default is `~/.local/share/ixf-toolbox/bin` on Unix and
  `%LOCALAPPDATA%\ixf-toolbox\bin` on Windows.
- Require `curl` plus `sha256sum` or `shasum` on Unix, and use built-in
  `Invoke-WebRequest` plus `Get-FileHash` on Windows; report a named missing-tool error.

The only test override is the pair `IXF_BOOTSTRAP_TESTING=1` and a non-empty
`IXF_BOOTSTRAP_RELEASE_BASE_URL`; production execution always uses the fixed HTTPS GitHub release
base. The tests must replace existing environment entries rather than appending duplicate variable
names, so an inherited proxy or `HOME` cannot override the fixture values.

The PowerShell test must also cover checksum mismatch, outside-`LOCALAPPDATA` rejection, conflicting
`-DryRun`/`-Apply`, and the no-network dry-run guarantee. If PowerShell is unavailable on a non-Windows
developer machine, skip those platform-specific tests with an explicit reason; the Windows CI job
must execute them.

- [ ] **Step 4: Implement the Claude-only SessionStart hook**

Use this cross-platform hook registration contract. The extensionless `session-start` script is
invoked through the checked-in polyglot `run-hook.cmd` dispatcher so Windows with Git Bash and
POSIX hosts use the same hook logic:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|clear|compact",
        "hooks": [
          {
            "type": "command",
            "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd\" session-start",
            "shell": "bash",
            "async": false
          }
        ]
      }
    ]
  }
}
```

The script must consume stdin and emit only:

```json
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "For i讯飞 / LarkShell links or document, sheets, bitable, OKR, and Messenger requests, consider the ixf-toolbox skills before choosing generic tools."
  }
}
```

`run-hook.cmd` must use the established polyglot layout: CMD executes the Windows branch, POSIX
shells consume that branch as a no-op and execute `bash <hook-dir>/<script-name>`, and the Windows
branch exits successfully with no context only when no Bash executable can be found. It must never
download software or modify PATH. Do not place hooks in the Codex package and do not add a `hooks`
field to its manifest.

Use this complete dispatcher content, retaining LF line endings:

```text
: <<'CMDBLOCK'
@echo off
if "%~1"=="" exit /b 1
set "HOOK_DIR=%~dp0"
if exist "C:\Program Files\Git\bin\bash.exe" (
  "C:\Program Files\Git\bin\bash.exe" "%HOOK_DIR%%~1" %2 %3 %4 %5 %6 %7 %8 %9
  exit /b %ERRORLEVEL%
)
if exist "C:\Program Files (x86)\Git\bin\bash.exe" (
  "C:\Program Files (x86)\Git\bin\bash.exe" "%HOOK_DIR%%~1" %2 %3 %4 %5 %6 %7 %8 %9
  exit /b %ERRORLEVEL%
)
where bash >nul 2>nul
if %ERRORLEVEL% equ 0 (
  bash "%HOOK_DIR%%~1" %2 %3 %4 %5 %6 %7 %8 %9
  exit /b %ERRORLEVEL%
)
exit /b 0
CMDBLOCK
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
SCRIPT_NAME="$1"
shift
exec bash "${SCRIPT_DIR}/${SCRIPT_NAME}" "$@"
```

- [ ] **Step 5: Regenerate and verify bootstrap behavior**

```bash
go run ./cmd/pluginpack
go run ./cmd/pluginpack --check
go test ./internal/pluginpack
```

Expected: PASS, including checksum mismatch refusal and dry-run no-write assertions.

- [ ] **Step 6: Commit bootstrap and hook support**

```bash
git add plugin-src internal/pluginpack plugins
git commit -m "feat: bootstrap plugin runtime on first use"
```

---

### Task 3: Native Plugin And Legacy Raw-Skill Diagnostics

**Files:**
- Create: `internal/agentinstall/diagnostics.go`
- Create: `internal/agentinstall/diagnostics_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`

**Interfaces:**
- Produces: `agentinstall.Diagnose(context.Context, agentinstall.Options) agentinstall.Report`.
- Consumes: `codex plugin list --json` and `claude plugin list --json` through an injected command runner.
- Produces: `agentRouting.installation.legacyRawSkills`, `nativePlugin`, `duplicateLoadRisk`, and `remediation`.

- [ ] **Step 1: Add failing unit tests for installed, absent, unknown, and duplicate states**

Use typed output and an injected runner:

```go
type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type Options struct {
	Home    string
	Timeout time.Duration
	Run     CommandRunner
}

type HostStatus struct {
	Status         string   `json:"status"`
	ID             string   `json:"id,omitempty"`
	Version        string   `json:"version,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Dir            string   `json:"dir,omitempty"`
	Paths          []string `json:"paths,omitempty"`
	InstalledCount int      `json:"installedCount,omitempty"`
	ExpectedCount  int      `json:"expectedCount,omitempty"`
}

type Report struct {
	LegacyRawSkills map[string]HostStatus `json:"legacyRawSkills"`
	NativePlugin    map[string]HostStatus `json:"nativePlugin"`
	DuplicateLoadRisk bool                `json:"duplicateLoadRisk"`
	Remediation     []string              `json:"remediation"`
}
```

Add these concrete fixture helpers to `internal/agentinstall/diagnostics_test.go`:

```go
func fixtureOptions(t *testing.T, codexJSON, claudeJSON string) agentinstall.Options {
	t.Helper()
	home := t.TempDir()
	return agentinstall.Options{
		Home:    home,
		Timeout: time.Second,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			command := strings.Join(append([]string{name}, args...), " ")
			switch command {
			case "codex plugin list --json":
				return []byte(codexJSON), nil
			case "claude plugin list --json":
				return []byte(claudeJSON), nil
			default:
				return nil, fmt.Errorf("unexpected command %q", command)
			}
		},
	}
}

func writeLegacySkill(t *testing.T, home, host, skillName string) string {
	t.Helper()
	hostDir := map[string]string{"codex": ".codex", "claude": ".claude"}[host]
	if hostDir == "" {
		t.Fatalf("unsupported legacy host %q", host)
	}
	directory := filepath.Join(home, hostDir, "skills", skillName)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "SKILL.md")
	if err := os.WriteFile(path, []byte("legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type treeEntry struct {
	Mode os.FileMode
	Data []byte
}

func snapshotTree(t *testing.T, root string) map[string]treeEntry {
	t.Helper()
	result := map[string]treeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not regular", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = treeEntry{Mode: info.Mode().Perm(), Data: data}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func cmpTree(before, after map[string]treeEntry) string {
	paths := map[string]bool{}
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	var sorted []string
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	for _, path := range sorted {
		oldValue, oldOK := before[path]
		newValue, newOK := after[path]
		if !oldOK || !newOK || oldValue.Mode != newValue.Mode || !bytes.Equal(oldValue.Data, newValue.Data) {
			return fmt.Sprintf("changed %s", path)
		}
	}
	return ""
}
```

Add the lifecycle-specific helper below to the same test file. It must run the extensionless
dispatcher through the platform shell rather than trying to execute it directly:

```go
func lifecycleRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func runLifecycleHook(t *testing.T, pluginRoot string, input []byte) []byte {
	t.Helper()
	dispatcher := filepath.Join(pluginRoot, "hooks", "run-hook.cmd")
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		cmdExe, err := exec.LookPath("cmd.exe")
		if err != nil {
			t.Skip("cmd.exe is required for the Windows hook lifecycle test")
		}
		command = exec.Command(cmdExe, "/d", "/c", dispatcher, "session-start")
	} else {
		bash, err := exec.LookPath("bash")
		if err != nil {
			t.Skip("bash is required for the POSIX hook lifecycle test")
		}
		command = exec.Command(bash, dispatcher, "session-start")
	}
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("session-start: %v\n%s", err, output)
	}
	if runtime.GOOS == "windows" && len(bytes.TrimSpace(output)) == 0 {
		t.Skip("Windows hook dispatcher found no supported Bash executable")
	}
	return output
}
```

The test imports are `bytes`, `context`, `encoding/json`, `fmt`, `io/fs`, `os`, `os/exec`,
`path/filepath`, `runtime`, `sort`, `strings`, `testing`, and `time`, plus
`github.com/serialq7ic4/ixf-toolbox/internal/agentinstall`. The production package uses the same
runner for both host commands and never writes command output into `HostStatus.Reason`.

Required test matrix:

```go
func TestDiagnoseNativePluginStates(t *testing.T) {
	tests := []struct {
		name       string
		codexJSON  string
		claudeJSON string
		codexWant  string
		claudeWant string
	}{
		{"installed", `{"installed":[{"pluginId":"ixf-toolbox@ixf-toolbox","version":"3.27.0"}]}`, `[{"id":"ixf-toolbox@ixf-toolbox","version":"3.27.0","enabled":true}]`, "installed", "installed"},
		{"absent", `{"installed":[]}`, `[]`, "not-installed", "not-installed"},
		{"different-plugin", `{"installed":[{"id":"other@catalog","version":"9.0.0"}]}`, `[{"pluginId":"other@catalog","version":"9.0.0"}]`, "not-installed", "not-installed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := Diagnose(context.Background(), fixtureOptions(t, test.codexJSON, test.claudeJSON))
			if report.NativePlugin["codex"].Status != test.codexWant || report.NativePlugin["claudeCode"].Status != test.claudeWant {
				t.Fatalf("report = %#v", report)
			}
		})
	}
}
```

Add this typed artifact, hook, host-status, and side-effect test after the state matrix:

```go
func TestNativePluginHostLifecycle(t *testing.T) {
	root := lifecycleRepoRoot(t)
	codexRoot := filepath.Join(root, "plugins", "codex", "ixf-toolbox")
	claudeRoot := filepath.Join(root, "plugins", "claude", "ixf-toolbox")
	type pluginManifest struct {
		Name    string          `json:"name"`
		Version string          `json:"version"`
		Skills  string          `json:"skills"`
		Hooks   json.RawMessage `json:"hooks"`
	}
	readManifest := func(path string) pluginManifest {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var manifest pluginManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return manifest
	}
	codex := readManifest(filepath.Join(codexRoot, ".codex-plugin", "plugin.json"))
	claude := readManifest(filepath.Join(claudeRoot, ".claude-plugin", "plugin.json"))
	if codex.Name != "ixf-toolbox" || claude.Name != codex.Name || codex.Version == "" || claude.Version != codex.Version {
		t.Fatalf("plugin manifests = %#v, %#v", codex, claude)
	}
	if codex.Skills != "./skills/" || codex.Hooks != nil || claude.Skills != "" || claude.Hooks != nil {
		t.Fatalf("host manifest boundaries = %#v, %#v", codex, claude)
	}

	type hookCommand struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Shell   string `json:"shell"`
		Async   bool   `json:"async"`
	}
	type hookRegistration struct {
		Matcher string        `json:"matcher"`
		Hooks   []hookCommand `json:"hooks"`
	}
	var hookConfig struct {
		Hooks map[string][]hookRegistration `json:"hooks"`
	}
	hookRaw, err := os.ReadFile(filepath.Join(claudeRoot, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(hookRaw, &hookConfig); err != nil {
		t.Fatal(err)
	}
	registrations := hookConfig.Hooks["SessionStart"]
	if len(registrations) != 1 || registrations[0].Matcher != "startup|clear|compact" || len(registrations[0].Hooks) != 1 {
		t.Fatalf("SessionStart registrations = %#v", registrations)
	}
	registration := registrations[0].Hooks[0]
	if registration.Type != "command" || !strings.Contains(registration.Command, "${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd") || registration.Async {
		t.Fatalf("SessionStart command = %#v", registration)
	}

	home := t.TempDir()
	homeBefore := snapshotTree(t, home)
	pluginBefore := snapshotTree(t, claudeRoot)
	codexJSON, err := json.Marshal(map[string]any{
		"installed": []map[string]string{{"pluginId": "ixf-toolbox@ixf-toolbox", "version": codex.Version}},
	})
	if err != nil {
		t.Fatal(err)
	}
	claudeJSON, err := json.Marshal([]map[string]any{{"id": "ixf-toolbox@ixf-toolbox", "version": claude.Version, "enabled": true}})
	if err != nil {
		t.Fatal(err)
	}
	report := agentinstall.Diagnose(context.Background(), agentinstall.Options{
		Home:    home,
		Timeout: time.Second,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			command := strings.Join(append([]string{name}, args...), " ")
			switch command {
			case "codex plugin list --json":
				return codexJSON, nil
			case "claude plugin list --json":
				return claudeJSON, nil
			default:
				return nil, fmt.Errorf("unexpected command %q", command)
			}
		},
	})
	if report.NativePlugin["codex"].Status != "installed" || report.NativePlugin["claudeCode"].Status != "installed" || report.DuplicateLoadRisk {
		t.Fatalf("native host report = %#v", report)
	}
	hookOutput := runLifecycleHook(t, claudeRoot, []byte("{}"))
	var hookPayload struct {
		Hook struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(hookOutput, &hookPayload); err != nil {
		t.Fatalf("decode hook output: %v\n%s", err, hookOutput)
	}
	if hookPayload.Hook.Event != "SessionStart" || hookPayload.Hook.Context != "For i讯飞 / LarkShell links or document, sheets, bitable, OKR, and Messenger requests, consider the ixf-toolbox skills before choosing generic tools." {
		t.Fatalf("hook payload = %#v", hookPayload)
	}
	pluginAfter := snapshotTree(t, claudeRoot)
	if diff := cmpTree(pluginBefore, pluginAfter); diff != "" {
		t.Fatalf("hook modified the Claude plugin package: %s", diff)
	}
	homeAfter := snapshotTree(t, home)
	if diff := cmpTree(homeBefore, homeAfter); diff != "" {
		t.Fatalf("host lifecycle modified HOME: %s", diff)
	}
}
```

Add a duplicate test that creates one legacy `SKILL.md` under each host and confirms only the host
with both a confirmed native plugin and a legacy file has `duplicateLoadRisk=true`; assert the
reported `paths` are the exact files and the pre-existing bytes remain unchanged. Also assert
command-not-found, timeout, non-zero exit, malformed JSON, and a valid-but-wrong schema return
`unknown` with one of the finite reasons `command-unavailable`, `timeout`, `command-failed`, or
`invalid-json`. The timeout fixture must block on `ctx.Done()` long enough to prove the two-second
context is passed through. Assert no raw stderr/stdout, URLs, or command arguments are copied into
the report.

- [ ] **Step 2: Run focused tests and confirm they fail**

```bash
go test ./internal/agentinstall ./cmd/ixf -run 'TestDiagnose|TestCollectDiagnosticsReportsAgentInstallation'
```

Expected: FAIL because the package and installation summary do not exist.

- [ ] **Step 3: Implement read-only host inspection**

Implementation rules:

- Use `exec.CommandContext` with a two-second default timeout.
- Mark a host `not-installed` only after its command returns valid JSON with no installed `ixf-toolbox` entry.
- Mark missing commands, timeout, process failure, and schema mismatch as `unknown` with a finite reason enum: `command-unavailable`, `timeout`, `command-failed`, or `invalid-json`.
- Recognize Codex's `{ "installed": [...] }` shape and Claude's top-level array shape. For each
  entry accept both `pluginId` (current host output) and `id` (older/compatible output), normalize
  an optional `@marketplace` suffix, and match the plugin name exactly as `ixf-toolbox`.
- Detect legacy raw skills by checking the seven known `SKILL.md` paths. Any non-zero count is `installed`; include counts so partial installs remain visible.
- Set `duplicateLoadRisk=true` only when the same host has a confirmed native plugin and at least one legacy raw skill.
- Never remove, rename, or rewrite a legacy directory.

Use a private typed decoder for host JSON and keep the public `HostStatus`/`Report` structs above as
the only serialized contract. The default runner must pass the context to
`exec.CommandContext`; it must not invoke a shell, parse JSON with `grep`, or persist host output.
When the injected runner returns `*exec.Error`, classify it as `command-unavailable`; classify
`context.DeadlineExceeded` as `timeout`, all other process errors as `command-failed`, and JSON
decode/schema errors as `invalid-json`.

- [ ] **Step 4: Integrate the report into root doctor without changing dependency fields**

Update `agentRoutingStatus(report agentinstall.Report)` to include the report. Preserve the
canonical `skills/*/SKILL.md` current-guidance path and the absence of the old top-level `skills`
object established in Task 1. Top-level `doctor.ok` remains based solely on the existing cookie
readiness result, with optional `dependencies.ok` separate. An `unknown` native-plugin status is
diagnostic information, not a doctor failure.

The JSON shape must include:

```json
{
  "agentRouting": {
    "installation": {
      "legacyRawSkills": {
        "codex": {"status": "installed"},
        "claudeCode": {"status": "not-installed"}
      },
      "nativePlugin": {
        "codex": {"status": "installed", "version": "3.27.0"},
        "claudeCode": {"status": "unknown", "reason": "command-unavailable"}
      },
      "duplicateLoadRisk": true,
      "remediation": []
    }
  }
}
```

Text output must summarize statuses without printing host CLI raw output.

- [ ] **Step 5: Prove doctor has no installation side effects**

Add `collectDiagnosticsWithOptions(cookiesPath string, agentOptions agentinstall.Options)` in
`cmd/ixf/main.go`; `collectDiagnostics` calls it with default agent options whose `Home` is the
current user home and whose runner invokes only `codex plugin list --json` and
`claude plugin list --json`. The function must pass `agentOptions` to `agentinstall.Diagnose` and
keep the existing dependency loader injection. Create a temporary HOME, snapshot its directory
tree with the local `snapshotHomeTree`/`compareHomeTree` helpers, run doctor with injected plugin-list
fixtures, and assert the before/after tree and file bytes are identical:

```go
func TestDoctorDoesNotInstallOrModifyAgentFiles(t *testing.T) {
	home := t.TempDir()
	emptyBin := filepath.Join(home, "empty-bin")
	if err := os.MkdirAll(emptyBin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyBin)
	originalReleaseLoader := dependencyReleaseLoader
	dependencyReleaseLoader = func(string, string) (ixfupdate.Release, error) {
		return ixfupdate.Release{
			TagName: "v" + version,
			HTMLURL: "https://github.example/releases/v" + version,
		}, nil
	}
	t.Cleanup(func() { dependencyReleaseLoader = originalReleaseLoader })
	before := snapshotHomeTree(t, home)
	_ = collectDiagnosticsWithOptions(filepath.Join(home, "cookies.json"), agentinstall.Options{
		Home: home,
		Run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "codex" {
				return []byte(`{"installed":[]}`), nil
			}
			return []byte(`[]`), nil
		},
	})
	after := snapshotHomeTree(t, home)
	if diff := compareHomeTree(before, after); diff != "" {
		t.Fatalf("doctor modified HOME:\n%s", diff)
	}
}
```

Add these independent helpers to `cmd/ixf/main_test.go` (do not import test helpers from
`internal/agentinstall`):

```go
type homeTreeEntry struct {
	Mode os.FileMode
	Data []byte
}

func snapshotHomeTree(t *testing.T, root string) map[string]homeTreeEntry {
	t.Helper()
	result := map[string]homeTreeEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not regular", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = homeTreeEntry{Mode: info.Mode().Perm(), Data: data}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func compareHomeTree(before, after map[string]homeTreeEntry) string {
	paths := map[string]bool{}
	for path := range before {
		paths[path] = true
	}
	for path := range after {
		paths[path] = true
	}
	sorted := make([]string, 0, len(paths))
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	for _, path := range sorted {
		oldValue, oldOK := before[path]
		newValue, newOK := after[path]
		if !oldOK || !newOK || oldValue.Mode != newValue.Mode || !bytes.Equal(oldValue.Data, newValue.Data) {
			return fmt.Sprintf("changed %s", path)
		}
	}
	return ""
}
```

Add `context`, `fmt`, `io/fs`, `sort`, and `internal/agentinstall` imports to `cmd/ixf/main_test.go`
as required by this test; preserve all existing tests and imports.

- [ ] **Step 6: Run tests and commit diagnostics**

```bash
go test ./internal/agentinstall ./cmd/ixf
git add internal/agentinstall cmd/ixf/main.go cmd/ixf/main_test.go
git commit -m "feat: diagnose native plugin installation"
```

---

### Task 4: Dependency Service And `ixf deps install`

**Files:**
- Create: `internal/dependencies/dependencies.go`
- Create: `internal/dependencies/dependencies_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`

**Interfaces:**
- Produces: `dependencies.Diagnose(dependencies.Options) map[string]any`.
- Produces: `dependencies.Install(dependencies.InstallOptions) (map[string]any, error)`.
- Produces CLI: `ixf deps install [--dry-run|--apply] [--cookies PATH] [--json]`.

- [ ] **Step 1: Add failing service and CLI tests**

Define injected process and release/status boundaries in `internal/dependencies/dependencies.go`:

```go
type Runner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, ...string) ([]byte, error)
}

type ReleaseLoader func(string, string) (update.Release, error)
type MermaidStatus func() map[string]any
type MessengerStatus func(string) map[string]any

type Options struct {
	CurrentVersion  string
	CookiesPath     string
	ReleaseLoader   ReleaseLoader
	MermaidStatus   MermaidStatus
	MessengerStatus MessengerStatus
	Runner          Runner
}

type InstallOptions struct {
	Options
	Apply bool
}
```

The production runner must implement `LookPath` with `exec.LookPath` and `Run` with
`exec.CommandContext`; it must pass arguments directly without a shell and must not expose command
output in the returned status payload. The package must provide a private `diagnose(options,
includeUpdate bool)` helper: exported `Diagnose` calls it with `includeUpdate=true`, while
`Install` calls it with `false` before and after mutation so planning a dependency repair never
performs a GitHub release request.

When a callback is nil, production code uses `docspublish.MermaidDependencyStatus`, the existing
Messenger doctor projection, and `update.LoadRelease`; tests always inject callbacks so no network,
browser, npm, or npx process is used. `Diagnose` returns all three keys `mermaid`, `messenger`, and
`update`. `Install` sets the update projection to exactly
`{"ok":true,"checked":false,"reason":"not-used-by-installer"}` and never calls the release
loader. Before `Apply=true` execution, preflight every selected executable with `LookPath`; if any
is missing, return a named remediation without running a partial command sequence.

The installer allowlist is exact and immutable:

```text
npm install -g @mermaid-js/mermaid-cli
npx puppeteer browsers install chrome-headless-shell
```

The service must preserve command order, report `applied=false` for an empty plan, and return a
structured failure identifying the allowlisted command if execution fails. It must not install
Chrome, modify Messenger profiles or cookies, configure a proxy, or repair login state.

Add these isolated service tests before wiring the CLI:

```go
func TestInstallDryRunPlansWithoutRunningCommandsOrLoadingRelease(t *testing.T) {
	var runCalls int
	var releaseCalls int
	status := func() map[string]any {
		return map[string]any{"ok": false, "available": false, "ready": false}
	}
	result, err := Install(InstallOptions{Options: Options{
		CurrentVersion: "1.2.3",
		ReleaseLoader: func(string, string) (update.Release, error) {
			releaseCalls++
			return update.Release{}, nil
		},
		MermaidStatus: status,
		MessengerStatus: func(string) map[string]any {
			return map[string]any{"ok": false}
		},
		Runner: fakeRunner{runCalls: &runCalls},
	}, Apply: false})
	if err != nil {
		t.Fatal(err)
	}
	if result["dryRun"] != true || result["apply"] != false || result["applied"] != false {
		t.Fatalf("result = %#v", result)
	}
	if runCalls != 0 || releaseCalls != 0 {
		t.Fatalf("dry-run side effects: run=%d release=%d", runCalls, releaseCalls)
	}
	dependencies := result["dependencies"].(map[string]any)
	if dependencies["update"].(map[string]any)["checked"] != false {
		t.Fatalf("update was checked: %#v", dependencies["update"])
	}
}

func TestInstallApplyRunsOnlyAllowlistedCommandsInOrder(t *testing.T) {
	var calls []string
	statuses := 0
	runner := fakeRunner{calls: &calls}
	result, err := Install(InstallOptions{Options: Options{
		MermaidStatus: func() map[string]any {
			statuses++
			ready := statuses >= 2
			return map[string]any{"ok": ready, "available": ready, "ready": ready}
		},
		MessengerStatus: func(string) map[string]any { return map[string]any{"ok": true} },
		Runner: runner,
	}, Apply: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"npm install -g @mermaid-js/mermaid-cli",
		"npx puppeteer browsers install chrome-headless-shell",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	if result["applied"] != true {
		t.Fatalf("result = %#v", result)
	}
	if statuses != 2 {
		t.Fatalf("Mermaid status calls = %d, want one preflight and one post-install probe", statuses)
	}
}
```

Use this complete test runner and add `context`, `reflect`, and the update package imports:

```go
type fakeRunner struct {
	calls    *[]string
	runCalls *int
	missing  map[string]error
	failAt   string
}

func (r fakeRunner) LookPath(name string) (string, error) {
	if err := r.missing[name]; err != nil {
		return "", err
	}
	return "/fixture/bin/" + name, nil
}

func (r fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.runCalls != nil {
		(*r.runCalls)++
	}
	call := strings.Join(append([]string{name}, args...), " ")
	if r.calls != nil {
		*r.calls = append(*r.calls, call)
	}
	if call == r.failAt {
		return []byte("private fixture output"), errors.New("fixture command failed")
	}
	return nil, nil
}
```

Add a separate test that a missing executable or a failing command returns an error, does not run a
later command, and never includes captured private output in the error payload. The CLI tests must
inject `dependencyDiagnose`/`dependencyInstall` function variables (restored with `t.Cleanup`) so
they never call npm, npx, GitHub, mmdc, or Messenger during unit tests.

Rename the existing tests to target the new command and add negative command-surface checks:

```go
func TestDepsInstallDryRunPlansMermaidToolchain(t *testing.T) {
	original := dependencyInstall
	var captured dependencies.InstallOptions
	dependencyInstall = func(options dependencies.InstallOptions) (map[string]any, error) {
		captured = options
		return map[string]any{
			"ok":      true,
			"dryRun":  true,
			"apply":   false,
			"applied": false,
			"dependencies": map[string]any{
				"update": map[string]any{"ok": true, "checked": false},
			},
		}, nil
	}
	t.Cleanup(func() { dependencyInstall = original })
	stdout, stderr, code := runCLITest(t, "deps", "install", "--dry-run", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	payload := decodeCLIJSON(t, stdout)
	if payload["dryRun"] != true || payload["apply"] != false || payload["applied"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["dependencies"].(map[string]any)["update"].(map[string]any)["checked"] != false {
		t.Fatalf("dry-run unexpectedly checked update release: %#v", payload["dependencies"])
	}
	if captured.Apply {
		t.Fatalf("captured options requested apply: %#v", captured)
	}
}

func TestRemovedSetupAndUpdateSkillsCommandsFailClosed(t *testing.T) {
	for _, args := range [][]string{{"setup", "skills"}, {"setup", "deps"}, {"update", "skills"}} {
		_, _, code := runCLITest(t, args...)
		if code != 2 {
			t.Fatalf("run(%v) code = %d, want 2", args, code)
		}
	}
}
```

Add `internal/dependencies` to the imports of `cmd/ixf/main_test.go`. Add a CLI apply test using
the same seam that asserts `Apply=true`, and a mode-conflict test that proves `--dry-run --apply`
returns exit code 2 without invoking `dependencyInstall`. Add a root-help assertion that `deps` is
listed and `setup` is absent.

- [ ] **Step 2: Run tests and confirm they fail**

```bash
go test ./internal/dependencies ./cmd/ixf -run 'TestDepsInstall|TestRemovedSetup|TestDependency'
```

Expected: FAIL because `internal/dependencies` and the `deps install` command surface do not exist;
the old implementation still exposes `setup deps`.

- [ ] **Step 3: Move dependency behavior into `internal/dependencies`**

Move Mermaid, Messenger, update reachability aggregation, install planning, process execution, and
output payload construction out of `cmd/ixf/main.go`. Add package-level seams in `cmd/ixf/main.go`:

```go
var dependencyDiagnose = dependencies.Diagnose
var dependencyInstall = dependencies.Install
```

Restore both variables in every CLI test with `t.Cleanup`. `Diagnose` preserves these root doctor
fields exactly:

```text
dependencies.ok
dependencies.mermaid
dependencies.messenger
dependencies.update
```

The installer may run only these commands:

```text
npm install -g @mermaid-js/mermaid-cli
npx puppeteer browsers install chrome-headless-shell
```

`Install` must return the plan without executing commands when `Apply=false`; when `Apply=true`,
execute the allowlisted commands in order and rerun Mermaid/Messenger diagnostics afterward while
leaving the update projection unchecked. Do not add automatic Chrome, Messenger profile, cookie, or
proxy repair. `runDeps` must pass `CurrentVersion`, `CookiesPath`, and the existing release-loader
test seam into the service, and must pass `Apply` exactly as parsed.

- [ ] **Step 4: Replace the command surface and remove setup**

Root help must contain:

```text
deps       Inspect or install optional local dependencies.
doctor     Inspect local Toolbox and dependency readiness without changing it.
```

`runDeps` supports only `install`; `deps --help` and `deps install --help` print usage to stdout and
exit 0. Remove `runSetup`, `runSetupDeps`, and all setup rows. `runUpdate` supports only `check` and
`self`. Update the update remediation to end at `ixf update self --apply --json`; never suggest
skill refresh. Unknown `deps` subcommands and all removed `setup`/`update skills` invocations exit
2, while dependency execution failures exit 1.

Mode rules:

- No mode flag is equivalent to dry-run.
- Explicit `--dry-run` is accepted.
- `--dry-run --apply` exits 2 without executing anything.
- Only `--apply` may execute npm/npx.

- [ ] **Step 5: Run package and full CLI tests**

```bash
go test ./internal/dependencies ./cmd/ixf
go test ./...
```

Expected: PASS, including doctor field compatibility and explicit apply-only process markers.

- [ ] **Step 6: Commit the dependency boundary**

```bash
git add internal/dependencies cmd/ixf/main.go cmd/ixf/main_test.go
git commit -m "feat: separate dependency diagnosis and install"
```

---

### Task 5: Skill Runtime Discovery And First-Use Guidance

**Files:**
- Modify: `skills/using-ixf-toolbox/SKILL.md`
- Modify: `skills/ixf-docs-reader/SKILL.md`
- Modify: `skills/ixf-docs-writer/SKILL.md`
- Modify: `skills/ixf-okr-reader/SKILL.md`
- Modify: `skills/ixf-okr-writer/SKILL.md`
- Modify: `skills/ixf-messenger-reader/SKILL.md`
- Modify: `skills/ixf-messenger-writer/SKILL.md`
- Modify generated: `plugins/codex/ixf-toolbox/skills/**`
- Modify generated: `plugins/claude/ixf-toolbox/skills/**`
- Modify: `repository_contract_test.go`

**Interfaces:**
- Consumes: `runtime.json` and packaged bootstrap scripts relative to each plugin root.
- Produces: identical host skill content with a shared runtime resolution and confirmation contract.

The package generator emits the bootstrap scripts under both plugin roots. A skill resolves its
own root by walking from its installed `SKILL.md`; it never assumes a repository checkout, a fixed
cache path, or a host-specific environment variable. The Codex package uses the same relative
layout as Claude, so the runtime-resolution text remains byte-identical across both hosts.

- [ ] **Step 1: Add failing routing and bootstrap contract tests**

Update repository tests to read canonical skills and generated copies. Require the routing description to contain all high-signal triggers:

```go
var routingTriggers = []string{
	"i讯飞", "讯飞文档", "LarkShell", "/docx/", "/wiki/", "/base/",
	"docx", "wiki", "sheets", "bitable", "OKR", "Messenger",
	"read", "publish", "update", "patch", "append row", "upload image", "attachment",
}
```

Require every skill to state:

```text
Resolve the Go `ixf` runtime before running a business command.
Do not download or install anything without explicit user confirmation.
Run the packaged bootstrap with `--dry-run` before `--apply`.
Run `ixf doctor --json` after bootstrap succeeds.
Use `ixf deps install`, not bootstrap, for Mermaid dependencies.
```

- [ ] **Step 2: Run repository contracts and confirm they fail**

```bash
go test . -run 'TestCanonicalSkills|TestRoutingSkillTriggers|TestSkillRuntimeBootstrap'
```

Expected: FAIL because current skills assume `ixf` already exists and the route description lacks URL/write triggers.

- [ ] **Step 3: Add the shared runtime resolution workflow to all skills**

Each skill must instruct the agent to:

1. Prefer `ixf` found on `PATH`.
2. Resolve the plugin root from the current packaged skill path with exactly
   `filepath.Dir(filepath.Dir(filepath.Dir(skillFile)))`: for
   `<plugin-root>/skills/<skill-name>/SKILL.md`, walk three directory levels above the file. Read
   `<plugin-root>/runtime.json` and use the sibling `<plugin-root>/scripts/bootstrap-runtime.sh` or
   `bootstrap-runtime.ps1`. Do not assume the process current directory or a repository checkout.
3. If `ixf` is absent, inspect the documented user-local bootstrap path for the current OS.
4. Compare `ixf --version` against `runtime.json.minimumVersion`.
5. If missing or too old, identify the packaged bootstrap script from the resolved plugin root.
6. Run bootstrap dry-run and show version, platform, release URL host, and target path.
7. Ask for explicit confirmation.
8. Run bootstrap apply only after confirmation, then invoke the installed path directly.
9. Run `ixf doctor --json` and continue the original task.

The skills must not claim that plugin installation itself downloaded the binary, and must not silently invoke `deps install`.

- [ ] **Step 4: Strengthen `using-ixf-toolbox` discovery without routing ordinary local Markdown**

Use this frontmatter description, which starts with the required trigger phrase and stays within the
skill frontmatter length limit:

```yaml
---
name: using-ixf-toolbox
description: Use when a request mentions i讯飞, 讯飞文档, or LarkShell and involves /docx/, /wiki/, /base/, docx, wiki, sheets, bitable, OKR, or Messenger, including read, publish, update, patch, append-row, image upload, attachment upload, or message workflows; do not use for ordinary local Markdown reading or editing.
---
```

Keep the existing local Markdown intent boundary, Go-only rule, dry-run-first writes, and domain routing behavior.

- [ ] **Step 5: Regenerate packages and verify exact skill parity**

```bash
go run ./cmd/pluginpack
go run ./cmd/pluginpack --check
go test .
```

Expected: PASS. The generated Codex and Claude `SKILL.md` files must be byte-identical to canonical files.

- [ ] **Step 6: Commit the skill routing changes**

```bash
git add skills plugins repository_contract_test.go
git commit -m "feat: guide plugin runtime bootstrap"
```

---

### Task 6: User Documentation And Migration Contract

**Files:**
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `AGENTS.md`
- Modify: `SECURITY.md`
- Modify: `docs/agent-routing.md`
- Modify: `docs/go-python-parity.md`
- Modify: `docs/python-removal-readiness.md`
- Modify: `docs/python-api-sunset.md`
- Modify: `docs/migration-from-legacy.md`
- Modify: `docs/release.md`
- Modify: `docs/supported-platforms.md`
- Modify: `repository_contract_test.go`

**Interfaces:**
- Produces: one current installation story per host and one explicit migration story for legacy raw skills.
- Preserves: historical commands only inside `CHANGELOG.md`, `docs/superpowers/` historical records,
  and the explicitly marked `Removed commands` section of `docs/migration-from-legacy.md`.

- [ ] **Step 1: Add failing documentation consistency contracts**

Define current-guidance files and reject removed commands in them. The list intentionally excludes
`docs/migration-from-legacy.md`, whose purpose is to name the removed commands as historical
migration references:

```go
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
```

Require both READMEs to contain native Codex/Claude install commands, bootstrap confirmation,
`ixf deps install --dry-run`, `ixf deps install --apply`, and the legacy raw-skill non-deletion
warning. Test `docs/migration-from-legacy.md` separately: it may name `ixf setup skills`,
`ixf setup deps`, and `ixf update skills` only inside a clearly marked removed-command section and
must state that native plugin installation is the replacement.

For every file in `currentGuidanceFiles`, reject the stale source paths
`skills/codex`, `skills/claude-code`, and `skills/*/*/SKILL.md`, as well as each removed command.
The test must not reject the canonical `skills/*/SKILL.md` path or historical Python terminology
that is used only to record the completed removal decision.

- [ ] **Step 2: Run documentation contracts and confirm they fail**

```bash
go test . -run 'TestCurrentGuidance|TestReadmeNativePlugin|TestReleaseDocumentation'
```

Expected: FAIL on the old setup/update instructions.

- [ ] **Step 3: Rewrite installation and dependency documentation**

Make plugin-first installation the first path in both READMEs:

```bash
codex plugin marketplace add serialq7ic4/ixf-toolbox
codex plugin add ixf-toolbox@ixf-toolbox
```

```bash
claude plugin marketplace add serialq7ic4/ixf-toolbox
claude plugin install ixf-toolbox@ixf-toolbox --scope user --yes
```

Explain that first use checks for `ixf`, presents a bootstrap dry-run, asks before installation, verifies checksum, installs in a user directory, and does not modify PATH. Keep a manual GitHub Release binary installation section only as troubleshooting/fallback for bootstrap failure, not as a second mandatory install phase.

Document dependency boundaries with:

```bash
ixf doctor --json
ixf deps install --dry-run --json
ixf deps install --apply --json
```

- [ ] **Step 4: Rewrite lifecycle and migration documentation**

Document independent lifecycle ownership:

```text
Codex/Claude plugin commands: skill discovery, update, disable, uninstall
ixf update check/self: Go runtime update
ixf doctor: read-only status
ixf deps install: confirmed optional dependency repair
```

State that existing `~/.codex/skills/ixf-*`, `~/.claude/skills/ixf-*`, and `using-ixf-toolbox` directories are legacy. The migration procedure is: install native plugin, start a new session, verify routing, inspect doctor duplicate risk, then let the user remove old directories manually. Do not provide an automatic delete command.

Update `AGENTS.md`, `docs/agent-routing.md`, and `docs/go-python-parity.md` to use the canonical
`skills/*/SKILL.md` source path and the generated host package paths where relevant. Remove setup
from current runtime wording, make the native Codex/Claude plugin lifecycle authoritative, and
fix `SECURITY.md` so issue reports ask for OS and `ixf --version`, not Python version.

Rewrite the current-state portions of `docs/python-removal-readiness.md` and
`docs/python-api-sunset.md` so they describe the shipped Go-only runtime and native plugin
installation as the supported path. They must explicitly say that the plugin resolves or
bootstraps the Go `ixf` executable only after user confirmation, that `ixf doctor --json` is
read-only, and that `ixf deps install --apply` is the only optional dependency mutation path.
Keep the Python references in these two files limited to historical deletion evidence; remove any
present-tense instruction that asks a user to install or invoke Python tooling.

The consistency test must not reject historical command names inside `docs/migration-from-legacy.md`
when they are inside the `Removed commands` section; it must reject those names as executable
instructions in every other current-guidance file and require the migration document to contain the
literal markers `Removed commands`, `native plugin`, and `do not delete automatically`. The test
must also scan `docs/python-removal-readiness.md` and `docs/python-api-sunset.md` as current guidance,
so neither can retain stale `setup` installation instructions.

- [ ] **Step 5: Correct release and platform docs while touching this surface**

Remove the stale `-X main.version` build example from `docs/release.md`; version remains embedded from `VERSION`. Replace release smoke expectations for `setup skills` with plugin-package checks and `deps install --dry-run`. Document the Unix and Windows user-local bootstrap paths in `docs/supported-platforms.md`.

- [ ] **Step 6: Run contracts and commit documentation**

```bash
go test .
git diff --check
git add README.md README.en.md AGENTS.md SECURITY.md docs repository_contract_test.go
git commit -m "docs: document native plugin lifecycle"
```

---

### Task 7: CI, Smoke Tests, Version 3.27.0, And Release Readiness

**Files:**
- Modify: `scripts/smoke-go-binary.sh`
- Create: `scripts/smoke-native-plugins.sh`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `repository_contract_test.go`
- Modify: `VERSION`
- Modify: `CHANGELOG.md`
- Modify generated: `.agents/plugins/marketplace.json`
- Modify generated: `.claude-plugin/marketplace.json`
- Modify generated: `plugins/codex/ixf-toolbox/**`
- Modify generated: `plugins/claude/ixf-toolbox/**`

**Interfaces:**
- Produces: CI/release gates for generated-package drift and plugin/bootstrap contracts.
- Produces: release candidate version `3.27.0` across binary and both plugin manifests.

- [ ] **Step 1: Add failing release contract assertions**

Require workflows to run:

```text
go run ./cmd/pluginpack --check
go test ./...
go vet ./...
```

Require `scripts/smoke-go-binary.sh` to execute `ixf deps install --dry-run --json` and reject any setup-skill install. Require release artifacts to remain exactly platform binaries plus `ixf_3.27.0_checksums.txt`.

Add `TestNativePluginArtifacts` to `repository_contract_test.go`. It must decode the generated
Codex and Claude manifests and marketplaces with `encoding/json`, assert both plugin names and
versions equal `VERSION`, assert the Codex manifest has `skills: "./skills/"` and no `hooks`, assert
the Claude package has `hooks/hooks.json` and seven root skill directories, and assert the two
marketplace source paths are `./plugins/codex/ixf-toolbox` and `./plugins/claude/ixf-toolbox`.
This test is the JSON parser used by the portable smoke script; no shell `grep` is allowed to
validate JSON.

- [ ] **Step 2: Update smoke scripts and workflows**

`scripts/smoke-native-plugins.sh` must:

- Run `go run ./cmd/pluginpack --check`.
- Run `go test . -run '^TestNativePluginArtifacts$' -count=1` to parse both manifests and
  marketplaces through the repository contract, not grep-based JSON parsing.
- Run both bootstrap scripts in dry-run mode when their shell is available.
- Run `claude plugin validate --strict plugins/claude/ixf-toolbox` only when `claude` is installed.
- Avoid writing outside a temporary HOME.

Add package drift checks before tests in CI and before binary builds in release. Do not make GitHub CI depend on Codex or Claude being installed; repository contracts remain the portable manifest gate.

- [ ] **Step 3: Bump the feature version and add release notes**

Set:

```text
3.27.0
```

Add `## 3.27.0 - 2026-09-10` to `CHANGELOG.md`, covering:

- Native Codex and Claude plugin marketplaces and lifecycle.
- Confirmed checksum-verified user-local runtime bootstrap.
- Canonical skills and host-specific generated packages.
- Claude-only SessionStart routing hint.
- Read-only native plugin/legacy raw-skill doctor diagnostics.
- `ixf deps install` replacing setup dependency commands.
- Removal of `setup skills`, `update skills`, and the setup family.

Regenerate all package metadata after changing `VERSION`:

```bash
go run ./cmd/pluginpack
go run ./cmd/pluginpack --check
```

- [ ] **Step 4: Run the complete release candidate verification**

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
scripts/smoke-native-plugins.sh
git diff --check
```

Expected: every command exits 0. `ixf --version` reports `3.27.0`; generated manifests also report `3.27.0`.

- [ ] **Step 5: Commit the release candidate**

```bash
git add VERSION CHANGELOG.md .github scripts repository_contract_test.go \
  .agents/plugins/marketplace.json .claude-plugin/marketplace.json plugins
git commit -m "chore: prepare v3.27.0 release"
```

---

### Task 8: Native Host Acceptance, Push, Release, And Local Update

**Files:**
- No source edits expected; any defect found here returns to the owning task and receives its own focused test and commit.

**Interfaces:**
- Consumes: clean `main`, local Codex and Claude Code CLIs, and GitHub through `127.0.0.1:7890`.
- Produces: pushed `main`, tag `v3.27.0`, GitHub Release, updated local binary, and native host plugin installations.

- [ ] **Step 1: Verify repository and release state before network mutation**

```bash
git status --short
git log --oneline --decorate origin/main..HEAD
test "$(cat VERSION)" = "3.27.0"
go run ./cmd/pluginpack --check
go test ./...
go vet ./...
```

Expected: clean worktree, intended design/feature commits only, version `3.27.0`, and all checks passing.

- [ ] **Step 2: Validate both native plugin lifecycles in isolated homes**

Use temporary HOME directories and the local repository marketplace:

```bash
scripts/smoke-native-plugins.sh --host-lifecycle
```

The smoke must verify:

- `codex plugin list --json` reports installed `ixf-toolbox`.
- `claude plugin list --json` reports installed and enabled `ixf-toolbox`.
- Codex package has no SessionStart hook.
- Claude's hook emits the short routing hint.
- Bootstrap dry-run writes no binary.
- No files are created under the real HOME.

- [ ] **Step 3: Push main through the required proxy and wait for CI**

Do not persist the proxy in git configuration or with shell-wide `export` statements. Prefix every
GitHub network command with all three proxy variables, including nested `gh run list` calls:

```bash
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
git push origin main

HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
gh run list --workflow CI --branch main --limit 1

ci_run_id="$(
  HTTP_PROXY=http://127.0.0.1:7890 \
  HTTPS_PROXY=http://127.0.0.1:7890 \
  ALL_PROXY=http://127.0.0.1:7890 \
  gh run list --workflow CI --branch main --limit 1 --json databaseId --jq '.[0].databaseId'
)"
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
gh run watch "${ci_run_id}" --exit-status
```

Expected: push succeeds and the macOS/Windows CI matrix passes. Do not create the release tag if CI fails.

- [ ] **Step 4: Tag and verify the GitHub Release through the proxy**

Keep the same per-command proxy rule for the tag push and every `gh` request:

```bash
git tag -a v3.27.0 -m "v3.27.0"
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
git push origin v3.27.0

release_run_id="$(
  HTTP_PROXY=http://127.0.0.1:7890 \
  HTTPS_PROXY=http://127.0.0.1:7890 \
  ALL_PROXY=http://127.0.0.1:7890 \
  gh run list --workflow Release --limit 1 --json databaseId --jq '.[0].databaseId'
)"
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
gh run watch "${release_run_id}" --exit-status

HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
gh release view v3.27.0 --json tagName,name,assets,url
```

Expected: release workflow passes and assets contain five platform binaries plus `ixf_3.27.0_checksums.txt`, with no Python or plugin archive artifacts.

- [ ] **Step 5: Update the local runtime and install both native plugins**

Prefix commands that may contact GitHub with the same proxy variables; do not rely on an exported
proxy remaining in the shell:

```bash
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
ixf update check --json
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
ixf update self --apply --json
ixf --version

HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
codex plugin marketplace add serialq7ic4/ixf-toolbox
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
codex plugin add ixf-toolbox@ixf-toolbox
codex plugin list --json

HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
claude plugin marketplace add serialq7ic4/ixf-toolbox
HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
claude plugin install ixf-toolbox@ixf-toolbox --scope user --yes
claude plugin list --json

HTTP_PROXY=http://127.0.0.1:7890 \
HTTPS_PROXY=http://127.0.0.1:7890 \
ALL_PROXY=http://127.0.0.1:7890 \
ixf doctor --json
```

Expected: local binary reports `3.27.0`, both hosts report the plugin, and doctor reports native installation statuses. If doctor reports `duplicateLoadRisk=true`, leave legacy raw skills untouched and report their paths for user-controlled cleanup after new-session validation.

- [ ] **Step 6: Validate routing in fresh Codex and Claude sessions**

Start one new session per host and test these intents without naming a skill:

```text
帮我读取这个 i讯飞 docx 链接
把这份 Markdown 发布到 i讯飞文档，先 dry-run
检查这个 /base/ 链接并规划新增一条带图片的记录
帮我看看这个普通本地 Markdown 文件
```

Expected: the first three discover ixf-toolbox routing; the local Markdown request uses the host filesystem. No remote write is applied during this acceptance test.
