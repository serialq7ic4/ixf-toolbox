# ixf-toolbox

**English** | [简体中文](README.md)

Let Codex, Claude Code, and other local coding agents read and publish authorized i讯飞/LarkShell cloud documents, and read or write confirmed OKR content.

> Built for local agent workflows. `ixf` is the unified local command. It reuses your desktop login session, runs no hosted service, sends no telemetry, and requires no Open Platform app.

<p>
  <img alt="python" src="https://img.shields.io/badge/Python-3.11%2B-3776AB">
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

The recommended path is to let the agent you are already using install Toolbox. Installation first installs the `ixf-toolbox` Python package, then registers the five skills into Codex or Claude Code.

### Codex

```bash
python -m pip install "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v1.2.0/ixf_toolbox-1.2.0-py3-none-any.whl"
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

### Claude Code

```bash
python -m pip install "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v1.2.0/ixf_toolbox-1.2.0-py3-none-any.whl"
ixf setup skills --runtimes claude-code --json
ixf --version
ixf doctor --json
```

### Both Agents

```bash
python -m pip install "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v1.2.0/ixf_toolbox-1.2.0-py3-none-any.whl"
ixf setup skills --runtimes auto --json
ixf --version
ixf doctor --json
```

On Windows, use `[windows]` instead of `[crypto]`:

```bash
python -m pip install "ixf-toolbox[windows] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v1.2.0/ixf_toolbox-1.2.0-py3-none-any.whl"
```

Windows cookie export is CI-tested but remains experimental on real desktop sessions.

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

### Go Migration Status

The v1.x line still uses the Python package as the default installation path and reference runtime. The Go CLI is migrating in stages: it now covers `--version`, `doctor`, `setup skills`, local `docs read/outline/chunk/inspect/cleanup`, authorized remote docx text/image/embedded-sheet reads, basic docx block-to-Markdown conversion, `update check/self`, checksum-verified Go binary self-update, and `update skills`; cookie export, full remote parity for wiki/mindnote, docs publish, and OKR read/write remain Python-reference behavior until golden parity is complete.

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

Add `--apply` after reviewing the planned changes. `--prune` is destructive and should only be used when removal is explicitly intended.

## Supported Scope

Toolbox currently supports:

- i讯飞/LarkShell `docx` document reading and Markdown conversion.
- Supported `wiki` links that resolve to known document types.
- Mindnote markers and embedded sheet TSV expansion exposed by supported document payloads.
- Simple tables, task lists, code languages, rich-text links, image block download, embedded sheet expansion, and safe artifact cleanup.
- Local Markdown chunking, reading, publishing, and test workflows.
- Authorized OKR reading, selected Objective writing, KR create/update, and publish-after-edit.
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
python -m pip install -e ".[crypto,dev]"
python -m compileall -q src
python -m pytest -q
python -m ruff check .
go test ./...
python -m build
scripts/smoke.sh
```
