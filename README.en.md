# ixf-toolbox

**English** | [简体中文](README.md)

Let Codex, Claude Code, and other local coding agents read and publish authorized i讯飞/LarkShell cloud documents, and read or write confirmed OKR content.

> Built for local agent workflows. `ixf` is the unified local command. It reuses your desktop login session, runs no hosted service, sends no telemetry, and requires no Open Platform app.

<p>
  <img alt="go" src="https://img.shields.io/badge/Go-1.24%2B-00ADD8">
  <img alt="platform" src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20experimental-lightgrey">
  <img alt="license" src="https://img.shields.io/badge/license-Apache%202.0-green">
</p>

`ixf-toolbox` provides one local CLI and five agent skills:

- `using-ixf-toolbox`: lightweight routing entry point for choosing the right document/OKR and reader/writer skill.
- `ixf-docs-reader`: read-only document reading, chunking, and image artifact handling.
- `ixf-docs-writer`: dry-run-first Markdown to docx publishing.
- `ixf-okr-reader`: read-only Objective / Key Result extraction from authorized OKR pages.
- `ixf-okr-writer`: dry-run-first creation or update of confirmed Objective / Key Result content.

This project is intentionally local and narrow. It is not a browser extension, daemon, sync service, bulk migration tool, or substitute for organizational data-access rules.

## Why This Exists

Private i讯飞/LarkShell documents and OKR pages are often inaccessible to coding agents through ordinary HTTP fetches. `ixf-toolbox` bridges that local workflow:

- The agent calls `ixf` through Codex / Claude Code skills.
- `ixf` reuses the desktop session you already authorized.
- Read workflows convert authorized content into local Markdown, TSV, image, and manifest artifacts for analysis.
- Write workflows produce dry-run plans by default and require explicit `--apply` before remote mutation.
- Cookies, diagnostics, generated artifacts, and input files stay local.

Compared with browser export tools, Toolbox is optimized for agent workflows:

| Project shape | Best for |
|---|---|
| Codex / Claude Code skill plus `ixf` | Local agent workflows for authorized docs, OKRs, and daily cross-functional work |
| Browser extension | In-browser one-click export, visual UI, PDF/HTML, and bulk attachment downloads |

## Install Into Codex / Claude Code

The recommended path is to let the agent you are already using install Toolbox. In v2.0, the default install path is the GitHub Release Go binary, followed by skill registration for Codex or Claude Code; a local Python environment is no longer required for new installs.

If you are using Codex, ask Codex directly:

> Install https://github.com/serialq7ic4/ixf-toolbox. Use the GitHub Release Go binary for the local `ixf` engine (macOS Apple Silicon: `ixf_3.0.0_darwin_arm64`, macOS Intel: `ixf_3.0.0_darwin_amd64`, Windows: `ixf_3.0.0_windows_amd64.exe`), then run `ixf setup skills --runtimes codex --json`, and verify with `ixf --version` and `ixf doctor --json`.

### macOS Apple Silicon

```bash
mkdir -p ~/.local/bin
curl -L -o ~/.local/bin/ixf \
  https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.0.0/ixf_3.0.0_darwin_arm64
chmod +x ~/.local/bin/ixf
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

For macOS Intel, use `ixf_3.0.0_darwin_amd64` instead.

### Windows PowerShell

```powershell
New-Item -ItemType Directory -Force $HOME\bin | Out-Null
Invoke-WebRequest -Uri https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.0.0/ixf_3.0.0_windows_amd64.exe -OutFile $HOME\bin\ixf.exe
$env:PATH = "$HOME\bin;$env:PATH"
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

### Both Agents

Use `--runtimes auto` instead of `--runtimes codex` to register both Codex and Claude Code skills.

### Go-only Runtime

Starting with v3.0, the repository no longer contains the Python runtime/package
implementation. The supported runtime is the Go `ixf` binary. Python may still
be used as this repository's test harness, but there is no Python package,
wheel, sdist, or Python API.

## Agent Usage

After installing the skills, ask your agent to work with authorized links or local files:

> Use using-ixf-toolbox to decide whether this link should use a document or OKR workflow, and whether it is read or write.

> Use ixf-docs-reader to read and summarize this document: https://tenant.example.test/wiki/example

> Use ixf-docs-writer to publish `notes/review.md` to `https://tenant.example.test`. Show the dry-run first and only apply after confirmation.

> Use ixf-okr-reader to read this OKR page and summarize objectives, key results, owners, and mentions.

> Use ixf-okr-writer to write my approved O3 and three KRs into this OKR page. Only modify O3 and show the dry-run plan first.

Before the first private remote read or write, make sure the local i讯飞/LarkShell desktop client is logged in.

## Commands

| Command | Purpose |
|---|---|
| `ixf docs read <source>...` | Read authorized cloud document links or local Markdown into Markdown, TSV, image, and manifest artifacts |
| `ixf docs outline <file.md>` | Build heading-aware dynamic reading metadata |
| `ixf docs chunk <file.md> --index <n>` | Print one dynamic Markdown chunk |
| `ixf docs inspect <source>` | Print a safe routing summary without reading content or printing full tokens |
| `ixf docs cleanup <out-dir>` | Remove generated read artifacts |
| `ixf docs publish <file.md>` | Publish Markdown as a new authorized docx document |
| `ixf okr read <url>` | Read an authorized OKR page as Markdown |
| `ixf okr write --url <url> --input <file.json>` | Create or update confirmed Objective / KR content |
| `ixf cookies export` | Export cookies from the local desktop session |
| `ixf doctor --json` | Inspect runtime, skills, and cookie metadata without printing cookie values |
| `ixf setup skills --runtimes auto --json` | Install Codex / Claude Code skills |
| `ixf update check --json` | Check the latest GitHub Release |
| `ixf update self --json` | Plan or apply a Toolbox package upgrade |
| `ixf update skills --runtimes auto --json` | Refresh installed skill wrappers |

### Runtime Status

Starting with v2.4, the Go binary owns the documented CLI runtime: document reads and publishing, OKR reads and writes, cookie export, doctor, skill setup, and update flows. Starting with v2.6, GitHub Releases publish only Go binaries and checksums. Starting with v3.0, the Python runtime/package implementation has been deleted.

## Manual Read Flow

```bash
ixf cookies export \
  --provider auto \
  --output /tmp/ixf_cookies.json

ixf doctor \
  --json \
  --cookies /tmp/ixf_cookies.json

ixf docs inspect \
  "https://tenant.example.test/wiki/example" \
  --json

ixf docs read \
  "https://tenant.example.test/wiki/example" \
  --cookies /tmp/ixf_cookies.json \
  --out-dir ./out \
  --expand-sheets \
  --download-images \
  --print-manifest
```

Generated Markdown, TSV, images, and manifests are local artifacts and should be treated as sensitive when the source is sensitive.

## Manual Write Flow

Write commands default to dry-run. Real remote mutation requires explicit `--apply`.

Publish Markdown:

```bash
ixf docs publish notes/review.md \
  --base-url https://tenant.example.test

ixf docs publish notes/review.md \
  --base-url https://tenant.example.test \
  --cookies /tmp/ixf_cookies.json \
  --apply
```

Write one OKR Objective by index:

```bash
ixf okr write \
  --url "https://tenant.example.test/okr/user/example/?okrId=example" \
  --input okr.json \
  --cookies /tmp/ixf_cookies.json \
  --objective-index 3
```

Add `--apply` after reviewing the planned changes. `--objective-index` updates only the selected Objective; when the target index is exactly one past the current Objective count, it creates that next Objective. Without `--objective-index`, the Go runtime matches Objectives by text and can write multiple Objectives. `--prune` is destructive and should only be used when removal is explicitly intended.

## Supported Scope

Toolbox currently supports:

- i讯飞/LarkShell `docx` document reading and Markdown conversion.
- Supported `wiki` links, including docx token resolution and bitable TSV output.
- Direct mindnote link reads, plus mindnote markers and embedded sheet TSV expansion exposed by supported document payloads.
- Simple tables, task lists, code languages, rich-text links, image block download, embedded sheet expansion, and safe artifact cleanup.
- Local Markdown chunking, reading, publishing, and test workflows.
- Authorized OKR reading, selected Objective update/create, multi-Objective writes by Objective text, KR create/update/order, explicit prune, and publish-after-edit.
- macOS and experimental Windows desktop-session cookie export, diagnostics, and skill installation.

Some cloud document blocks do not map perfectly to Markdown. The converter prioritizes agent analysis usefulness over visual fidelity.

## Platforms

| Platform | Status | Notes |
|---|---|---|
| macOS | Tier 1 | Reads the LarkShell Chromium profile and decrypts cookies with Keychain. |
| Windows | CI-tested / experimental | Reads the LarkShell Chromium profile and decrypts cookies with DPAPI; more live desktop validation is needed. |

Linux desktop-session export is not supported because i讯飞 does not ship a Linux desktop client. Pure parsing and dry-run components may still work when dependencies are available.

## Migration

The earlier reader and writer repositories are archived. New installs and future feature work should use `ixf-toolbox`.

See [`docs/migration-from-legacy.md`](docs/migration-from-legacy.md) for command mapping.

## Safety

- Cookie export reuses the local desktop login session.
- `doctor` does not print cookie values.
- Remote read errors do not echo raw API payloads.
- Remote writes default to dry-run and require explicit `--apply`.
- Destructive OKR pruning requires explicit `--prune`.
- Generated Markdown, TSV, images, manifests, and OKR JSON may contain private content.
- Do not commit cookies, generated artifacts, full private URLs, internal responses, or sensitive diagnostics.

See `SECURITY.md`, `PRIVACY.md`, and `CONTRIBUTING.md`.

## Development

```bash
git clone https://github.com/serialq7ic4/ixf-toolbox.git
cd ixf-toolbox
python -m pip install "pytest>=8.0" "ruff>=0.5"
python -m compileall -q tests scripts
python -m pytest -q
python -m ruff check .
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
```
