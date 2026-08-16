# ixf-toolbox

**English** | [简体中文](README.md)

Let Codex, Claude Code, and other local coding agents read and publish authorized i讯飞/LarkShell cloud documents, read or write confirmed OKR content, and use local Messenger workflows.

> Built for local agent workflows. `ixf` is the unified local command. It reuses your desktop login session, runs no hosted service, sends no telemetry, and requires no Open Platform app.

<p>
  <img alt="go" src="https://img.shields.io/badge/Go-1.24%2B-00ADD8">
  <img alt="platform" src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20experimental-lightgrey">
  <img alt="license" src="https://img.shields.io/badge/license-Apache%202.0-green">
</p>

`ixf-toolbox` provides one local CLI and seven agent skills:

- `using-ixf-toolbox`: lightweight routing entry point for choosing the right document/OKR/Messenger and reader/writer skill.
- `ixf-docs-reader`: read-only document reading, chunking, and image artifact handling.
- `ixf-docs-writer`: dry-run-first writes for publishing new docx files, inserting fragments under headings, or replacing an existing docx body; `publish` does not overwrite existing docx files.
- `ixf-okr-reader`: read-only Objective / Key Result extraction from authorized OKR pages.
- `ixf-okr-writer`: dry-run-first creation or update of confirmed Objective / Key Result content.
- `ixf-messenger-reader`: read-only Messenger readiness checks plus recent or unread conversation extraction.
- `ixf-messenger-writer`: dry-run-first approved sends with target verification and fresh-session post-send verification.

This project is intentionally local and narrow. It is not a browser extension, daemon, sync service, bulk migration tool, or substitute for organizational data-access rules.

## Why This Exists

Private i讯飞/LarkShell documents, OKR pages, and desktop Messenger are often inaccessible to coding agents through ordinary HTTP fetches. `ixf-toolbox` bridges that local workflow:

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

The recommended path is to let the agent you are already using install Toolbox. The default install path is the GitHub Release Go binary, followed by seven-skill registration for Codex or Claude Code; a local Python environment is not required.

If you are using Codex, ask Codex directly:

> Install https://github.com/serialq7ic4/ixf-toolbox. Use the GitHub Release Go binary for the local `ixf` engine (macOS Apple Silicon: `ixf_3.21.0_darwin_arm64`, macOS Intel: `ixf_3.21.0_darwin_amd64`, Windows: `ixf_3.21.0_windows_amd64.exe`), then run `ixf setup skills --runtimes codex --json`, and verify with `ixf --version` and `ixf doctor --json`.

### macOS Apple Silicon

```bash
mkdir -p ~/.local/bin
curl -L -o ~/.local/bin/ixf \
  https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.21.0/ixf_3.21.0_darwin_arm64
chmod +x ~/.local/bin/ixf
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

For macOS Intel, use `ixf_3.21.0_darwin_amd64` instead.

### Windows PowerShell

```powershell
New-Item -ItemType Directory -Force $HOME\bin | Out-Null
Invoke-WebRequest -Uri https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.21.0/ixf_3.21.0_windows_amd64.exe -OutFile $HOME\bin\ixf.exe
$env:PATH = "$HOME\bin;$env:PATH"
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

### Both Agents

Use `--runtimes auto` instead of `--runtimes codex` to register both Codex and Claude Code skills.

### Go-only Runtime

Starting with v3.1, the repository no longer contains the Python runtime/package
implementation or Python test harness. The supported runtime is the Go `ixf`
binary, and development, CI, and release checks use the Go toolchain.

All current document, wiki, docx, sheets, OKR, cookie, setup, update, and Messenger workflows use Go `ixf` only. Do not use Python fallback, and do not call the legacy `ixfdoc` or `ixfwrite` commands; historical changelog entries and `docs/superpowers/` plans are not current routing guidance. See [`docs/agent-routing.md`](docs/agent-routing.md) for the current routing contract; `ixf doctor --json` exposes `agentRouting` diagnostics.

## Agent Usage

After installing the skills, ask your agent to work with authorized links, local files, or Messenger requests. You do not need to name a specific skill. Describe the task naturally; `using-ixf-toolbox` routes in the background based on link type, read/write intent, and safety boundaries.

> Summarize this document: https://tenant.example.test/wiki/example

> Review this OKR page and summarize the objectives, key results, owners, and mentions relevant to me.

> Summarize the server SN list in this sheet.

> Write `cells.tsv` into this sheet starting at B2. Show the dry-run first and only apply after I confirm.

> Publish `notes/review.md` to `https://tenant.example.test`. Show the dry-run first and only apply after I confirm.

> Insert this table under the `1.1 Account initialization` section in this document. Show the dry-run plan first.

> Write my approved O3 and three KRs into this OKR page. Only modify O3 and show the dry-run plan first.

> Check unread messages and summarize what needs my attention.

> Send this message to the group. Show the dry-run plan first and wait for my confirmation.

Before the first private remote read or write, make sure the local i讯飞/LarkShell desktop client is logged in.

## Commands

| Command | Purpose |
|---|---|
| `ixf docs read <source>...` | Read authorized cloud document links or local Markdown into Markdown, TSV, image, and manifest artifacts |
| `ixf sheets read <sheets-url>` | Read a direct sheets link as Markdown/TSV content |
| `ixf sheets update --url <sheets-url> --range A1 --input cells.tsv --dry-run` | Plan a TSV cell update without writing |
| `ixf sheets update --url <sheets-url> --range A1 --input cells.tsv --apply` | Write confirmed TSV cell updates and verify by readback |
| `ixf bitable inspect --url <bitable-or-host-url> --json` | Inspect safe metadata for a direct bitable, wiki bitable, or docx embedded bitable |
| `ixf bitable read --url <bitable-or-host-url> --json` | Read a safe bitable summary; currently aliases `inspect` |
| `ixf bitable record create --url <bitable-or-host-url> --input row.json --dry-run --json` | Plan creating a bitable record appended to the current view by default, including local file paths for attachment fields, without writing |
| `ixf bitable record create --url <bitable-or-host-url> --input row.json --apply --json` | API-only creation of confirmed bitable records appended to the current view by default, with text fields, attachment upload, and readback verification |
| `ixf bitable attach --url <bitable-or-host-url> --field <attachment-field> --record-match Field=Value --file image.png --dry-run --json` | Plan a local image/attachment upload into a bitable attachment field without writing |
| `ixf docs outline <file.md>` | Build heading-aware dynamic reading metadata |
| `ixf docs chunk <file.md> --index <n>` | Print one dynamic Markdown chunk |
| `ixf docs inspect <source>` | Print a safe routing summary without reading content or printing full tokens |
| `ixf docs structure <doc-or-wiki-url> --json` | Print safe document structure preflight metadata, including heading paths, section ranges, and complex-block risk |
| `ixf docs cleanup <out-dir>` | Remove generated read artifacts |
| `ixf docs publish <file.md>` | Publish Markdown as a new authorized docx document; does not overwrite existing docx files |
| `ixf docs patch insert <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --dry-run` | Plan a non-destructive block insertion under a heading |
| `ixf docs table append-row --url <doc-or-wiki-url> --input row.json --dry-run --json` | Plan appending one row to an existing native docx table, including image cells |
| `ixf docs patch insert <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --apply` | Insert confirmed fragment blocks without replacing existing document body |
| `ixf docs patch replace-section <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --dry-run` | Plan replacing one heading section, rejecting complex blocks by default |
| `ixf docs patch delete-section --url <doc-or-wiki-url> --under-heading <heading> --dry-run` | Plan deleting one heading section, rejecting complex blocks by default |
| `ixf docs update <file.md> --url <docx-url> --dry-run` | Plan replacing an existing docx body without writing |
| `ixf docs update <file.md> --url <docx-url> --apply` | Replace an existing docx body, rejecting complex blocks by default; use `--allow-complex-replace` after confirmation |
| `ixf okr read <url>` | Read an authorized OKR page as Markdown |
| `ixf okr write --url <url> --input <file.json>` | Create or update confirmed Objective / KR content |
| `ixf messenger doctor --json` | Inspect Messenger desktop profile, browser, and cookie readiness |
| `ixf messenger open --to <target> --mode person\|conversation --dry-run --json` | Plan opening a person or conversation without sending |
| `ixf messenger open --to <target> --mode person\|conversation --apply --json` | Launch a cloned-profile browser and verify the target chat without sending |
| `ixf messenger read --scope unread\|recent --dry-run --json` | Plan unread or recent conversation reads without launching a browser |
| `ixf messenger read --scope unread\|recent --apply --json` | Launch a cloned-profile browser and read conversation snippets without sending |
| `ixf messenger send --to <target> --mode person\|conversation --message <text> --dry-run --json` | Plan a send without launching a browser or echoing the full message body |
| `ixf messenger send --to <target> --mode person\|conversation --message <text> --apply --json` | Send an approved message and report success only after fresh-session verification |
| `ixf cookies export` | Export cookies from the local desktop session |
| `ixf doctor --json` | Inspect runtime, skills, and cookie metadata without printing cookie values |
| `ixf setup skills --runtimes auto --json` | Install Codex / Claude Code skills |
| `ixf update check --json` | Check the latest GitHub Release |
| `ixf update self --json` | Plan or apply a Toolbox package upgrade |
| `ixf update skills --runtimes auto --json` | Refresh installed skill wrappers |

### Runtime Status

Starting with v2.4, the Go binary owns the documented CLI runtime: document reads and publishing, OKR reads and writes, cookie export, doctor, skill setup, and update flows. Starting with v2.6, GitHub Releases publish only Go binaries and checksums. Starting with v3.0, the Python runtime/package implementation has been deleted. Starting with v3.1, tests and release workflows no longer depend on Python. Starting with v3.3, Messenger begins a staged Go-native rollout. Starting with v3.4, it can open and verify a target chat under explicit --apply. Starting with v3.5, it can read unread or recent conversations. Starting with v3.6, it can send approved messages and requires fresh-session verification before reporting success. Starting with v3.7, Messenger has a GA runbook and actionable diagnostic remediation. Starting with v3.8, agent routing diagnostics and Messenger stability metadata are exposed through doctor commands. Starting with v3.9, existing-docx body replacement dry-run/preflight is available. Starting with v3.10, approved existing-docx body replacement writes are available. Starting with v3.11, complex-block explicit override and the update runbook are available. Starting with v3.13, dedicated `ixf sheets read` and `ixf sheets update --dry-run` command surfaces are available. Starting with v3.14, API-only `ixf sheets update --apply` writes cells and verifies by readback. Starting with v3.16, the docx block graph foundation is available. Starting with v3.17, `ixf docs patch insert --dry-run` is available. Starting with v3.18, approved API-only insert-under-heading writes verify unchanged existing blocks. Starting with v3.20, API-only bounded section replace/delete verifies outside-section blocks remain unchanged. Starting with v3.21, read/write preflight exposes safe heading, section, and complex-block structure summaries, and `ixf docs read --out-dir` writes `.structure.json` artifacts. Starting with v3.22, docs publishing can use a configured default tenant so natural publish requests do not stop at local-only drafts when `--base-url` is omitted. Starting with v3.25, `ixf docs table append-row` adds native docx table row dry-run/apply with PNG/JPEG/SVG image-cell upload and binding, and `ixf bitable` adds inspect/read, record create dry-run, API-only `record create --apply` for text/attachment record creation with readback verification, and attachment dry-run planning; `attach --apply` remains fail-closed until the existing-record update contract is captured.

See [`docs/agent-routing.md`](docs/agent-routing.md) for the agent routing contract. See [`docs/messenger.md`](docs/messenger.md) for Messenger operations, including Chrome/Chromium-only discovery, cloned profile isolation, read side effects, and send success criteria.

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

ixf sheets read \
  "https://tenant.example.test/sheets/example?sheet=sheet1" \
  --cookies /tmp/ixf_cookies.json

ixf docs read \
  "https://tenant.example.test/wiki/example" \
  "https://tenant.example.test/sheets/example?sheet=sheet1" \
  --cookies /tmp/ixf_cookies.json \
  --out-dir ./out \
  --expand-sheets \
  --download-images \
  --print-manifest
```

Generated Markdown, TSV, images, manifests, and `.structure.json` files are local artifacts and should be treated as sensitive when the source is sensitive. Remote docx/wiki reads include safe structure summaries in the manifest so agents can inspect heading paths, section ranges, duplicate headings, and complex-block risk before summarizing or writing.

## Manual Write Flow

Write commands default to dry-run. Real remote mutation requires explicit `--apply`.

Publish Markdown as a new docx:

If your tenant is stable, configure a default publish base URL first. Agents can
then run publish dry-runs for natural "publish to i讯飞 docs" requests even when
the prompt does not include an explicit URL:

```bash
export IXF_DOCS_DEFAULT_BASE_URL=https://tenant.example.test
```

Or write `~/.config/ixf-toolbox/config.json`:

```json
{"docs":{"defaultBaseURL":"https://tenant.example.test"}}
```

`ixf doctor --json` reports `docs.defaultBaseURL.configured/source/host` without
printing cookies or tokens.

Markdown `` ```mermaid `` fenced code, plus exported `` ```Plain `` blocks whose
first non-empty content line starts with Mermaid diagram keywords such as
`flowchart`, `sequenceDiagram`, or `erDiagram`, are written as docx image blocks
for `publish`, `update`, and `patch` instead of code blocks. Dry-runs report
`mermaidImageCount`, `plannedImageCount`, `mermaidRendererAvailable`,
`mermaidPreferredFormat`, and `mermaidFallbackFormat`. Real `--apply` requires
Mermaid CLI `mmdc` on `PATH`; the writer renders/uploads SVG first and falls
back to PNG if SVG rendering or upload fails. Missing `mmdc` fails clearly
before remote writes start.

```bash
ixf docs publish notes/review.md \
  --base-url https://tenant.example.test

ixf docs publish notes/review.md \
  --cookies /tmp/ixf_cookies.json \
  --apply
```

Insert content under a heading:

For localized additions such as "insert this table under section 1.1", use
`ixf docs patch insert`, not `ixf docs update`. Patch insert creates new blocks
and links them into the target heading's section without replacing the whole
body.

```bash
ixf docs patch insert fragment.md \
  --url https://tenant.example.test/wiki/example \
  --under-heading "1.1 Account initialization" \
  --cookies /tmp/ixf_cookies.json \
  --dry-run
```

After confirming `duplicateCandidate=false`, `existingBlocksTouched=false`,
`tableBlockType="table"`, and `tableFallbackCount=0`, apply:

```bash
ixf docs patch insert fragment.md \
  --url https://tenant.example.test/wiki/example \
  --under-heading "1.1 Account initialization" \
  --cookies /tmp/ixf_cookies.json \
  --require "critical content" \
  --apply
```

After apply, require `verify.ok=true` and `verify.unchangedExistingBlocks=true`.
If `duplicateCandidate=true`, stop instead of inserting again.

Replace or delete one section:

For requests like "replace this section" or "delete this section", use `ixf docs patch replace-section` or `ixf docs patch delete-section`. These commands only modify root-child references inside the target heading section and verify `verify.unchangedOutsideSectionBlocks=true` after apply. Simple additive edits should still use `ixf docs patch insert`; do not replace a section just to insert content.

```bash
ixf docs patch replace-section section.md \
  --url https://tenant.example.test/wiki/example \
  --under-heading "1.1 Account initialization" \
  --cookies /tmp/ixf_cookies.json \
  --dry-run
```

```bash
ixf docs patch delete-section \
  --url https://tenant.example.test/wiki/example \
  --under-heading "Obsolete section" \
  --cookies /tmp/ixf_cookies.json \
  --dry-run
```

If the target section contains complex content such as images, embedded sheets, or highlight blocks, the command rejects by default. Use `--allow-complex-section-replace` with explicit `--apply` only after accepting that those rich blocks will be replaced or deleted.

Update an existing docx:

`ixf docs update` uses `replace_body` mode: it keeps the original URL, permissions, and location while replacing body blocks. It rejects original documents containing complex blocks such as images or embedded sheets by default. See [`docs/docs-update.md`](docs/docs-update.md).

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --dry-run
```

After reviewing the dry-run plan:

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --require "critical content" \
  --apply
```

If dry-run reports complex blocks, use the override only after explicitly accepting that those blocks will be lost:

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --allow-complex-replace \
  --apply
```

Update sheet cells:

Direct sheets links use a separate command surface, not `ixf docs update`. Sheet write input is a TSV file, and `--range` is the top-left start cell. Start with a dry-run plan; real remote mutation requires explicit `--apply` and verifies target cells by readback.

```bash
ixf sheets update \
  --url "https://tenant.example.test/sheets/example?sheet=sheet1" \
  --range B2 \
  --input cells.tsv \
  --dry-run
```

After reviewing the plan:

```bash
ixf sheets update \
  --url "https://tenant.example.test/sheets/example?sheet=sheet1" \
  --range B2 \
  --input cells.tsv \
  --cookies /tmp/ixf_cookies.json \
  --apply
```

For an embedded sheet, keep `--url` on the direct sheets link and add `--host-url` for the parent docx/wiki link so the request carries the required host token:

```bash
ixf sheets update \
  --url "https://tenant.example.test/sheets/example?sheet=sheet1" \
  --host-url "https://tenant.example.test/docx/parent" \
  --range B2 \
  --input cells.tsv \
  --cookies /tmp/ixf_cookies.json \
  --apply
```

Append rows to native docs tables, including image cells:

Native tables inside docx documents are docs blocks, not bitable data. Use `ixf docs table append-row` for those tables. Use `ixf bitable` only for real `/base/...` tables or embedded bitables that resolve to a base token.

The input JSON maps fields to the first-row table headers. Text fields become text blocks; image fields use local file paths and currently support PNG, JPEG, and SVG. If the document has exactly one native table, `--table-index` can be omitted; documents with multiple tables require an explicit 1-based table index. Dry-run first, then use `--apply` to create the row, upload images to `docx_image`, bind image block tokens, and verify readback.

```json
{
  "fields": {
    "Issue": "native docs table append-row test",
    "Screenshot": { "file": "~/Downloads/ceph_logo.png" }
  }
}
```

```bash
ixf docs table append-row \
  --url "https://tenant.example.test/docx/dox_example" \
  --input row.json \
  --cookies /tmp/ixf_cookies.json \
  --dry-run \
  --json
```

```bash
ixf docs table append-row \
  --url "https://tenant.example.test/docx/dox_example" \
  --input row.json \
  --cookies /tmp/ixf_cookies.json \
  --apply \
  --json
```

Plan and create bitable attachment records:

Bitable attachment fields belong to the bitable data layer; do not modify them through `ixf docs update` or `ixf sheets update`. Direct bitable links, wiki bitable links, and docx embedded bitable links use `ixf bitable`. `record create --apply` supports API-only new records for text and attachment fields. `attach --apply` remains fail-closed until the existing-record update API contract is captured.

Use `record create` for new records. The input JSON can be a field map or a top-level `fields` object; attachment fields use local file paths, including absolute paths and `~/...`. New records append to the current view by default; pass `--insert-position top` to restore the old top-insert behavior, or `--insert-position bottom` to state the default explicitly. Dry-run first and inspect `insertPosition`, `plannedRecordIndex`, and attachment planning, then use `--apply` after review to write and verify `verify.recordIndex` by readback.

```json
{
  "fields": {
    "问题简述": "ixf dry-run record create test",
    "问题详情": "validate a new row with an image attachment",
    "截图": { "file": "~/Downloads/ceph_logo.jpeg" }
  }
}
```

```bash
ixf bitable record create \
  --url "https://tenant.example.test/base/bas_example?table=tbl_main&view=vew_grid" \
  --input row.json \
  --cookies /tmp/ixf_cookies.json \
  --dry-run \
  --json
```

```bash
ixf bitable record create \
  --url "https://tenant.example.test/base/bas_example?table=tbl_main&view=vew_grid" \
  --input row.json \
  --cookies /tmp/ixf_cookies.json \
  --apply \
  --json
```

```bash
ixf bitable attach \
  --url "https://tenant.example.test/base/bas_example?table=tbl_main&view=vew_grid" \
  --field "Screenshot" \
  --record-match "Title=Image bug" \
  --file ceph_logo.png \
  --cookies /tmp/ixf_cookies.json \
  --dry-run \
  --json
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
- Native docx table append-row dry-run/apply, including text cells and PNG/JPEG/SVG image-cell upload and binding.
- Safe bitable metadata inspection plus attachment-upload dry-run planning for direct bitable, wiki bitable, and docx embedded bitable links.
- Bitable record-create dry-run planning and API-only writes, including local file paths for attachment fields, asset upload, and readback verification; apply currently supports text and attachment fields.
- Direct mindnote and sheets link reads, plus mindnote markers and embedded sheet TSV expansion exposed by supported document payloads.
- Simple tables, task lists, code languages, rich-text links, image block download, direct sheets reads, embedded sheet expansion, sheets update dry-run/apply, and safe artifact cleanup.
- Local Markdown chunking, reading, publishing, and test workflows.
- Authorized OKR reading, selected Objective update/create, multi-Objective writes by Objective text, KR create/update/order, explicit prune, and publish-after-edit.
- Messenger readiness diagnostics, profile discovery, safe cloned profile usage, dry-run open planning, explicit --apply target verification, read-only recent/unread extraction, and approved sends with fresh-session verification.
- macOS and experimental Windows desktop-session cookie export, diagnostics, and skill installation.

Some cloud document blocks do not map perfectly to Markdown. The converter prioritizes agent analysis usefulness over visual fidelity.

## Platforms

| Platform | Status | Notes |
|---|---|---|
| macOS | Tier 1 | Reads the LarkShell Chromium profile, decrypts cookies with Keychain, discovers `profile_explorer` for Messenger, and auto-discovers only Chrome/Chromium for Messenger browser automation. |
| Windows | CI-tested / experimental | Reads the LarkShell Chromium profile and decrypts cookies with DPAPI; Messenger profile discovery needs more live desktop validation; Messenger browser automation auto-discovers only Chrome/Chromium. |

Linux desktop-session export is not supported because i讯飞 does not ship a Linux desktop client. Pure parsing and dry-run components may still work when dependencies are available. More Messenger details are in [`docs/messenger.md`](docs/messenger.md).

## Migration

The earlier reader and writer repositories are archived. New installs and future feature work should use `ixf-toolbox`.

See [`docs/migration-from-legacy.md`](docs/migration-from-legacy.md) for command mapping.

## Safety

- Cookie export reuses the local desktop login session.
- `doctor` does not print cookie values.
- Remote read errors do not echo raw API payloads.
- Remote writes default to dry-run and require explicit `--apply`.
- `ixf sheets update --apply` supports only confirmed TSV cell writes; do not use `ixf docs update` to modify sheet content.
- `ixf docs table append-row --apply` only mutates native docx table blocks; do not use it for bitable/base records or sheet cells.
- `ixf bitable record create --apply` only supports confirmed new records, appended to the current view by default; apply currently supports text and attachment fields and verifies by readback. Do not use `ixf docs update` or `ixf sheets update` to modify bitable data.
- `ixf bitable attach` currently supports dry-run planning only; ordinary docx `view/file` preview blocks are not bitable entry points.
- `ixf docs patch replace-section` and `ixf docs patch delete-section` are bounded destructive operations; simple additions must use `ixf docs patch insert` first.
- Messenger currently supports diagnostics, dry-run open planning, explicit --apply target verification, read-only conversation extraction, and approved sends with fresh-session verification. Messenger auto-discovers only Chrome/Chromium and always uses a cloned profile.
- Destructive OKR pruning requires explicit `--prune`.
- Generated Markdown, TSV, images, manifests, and OKR JSON may contain private content.
- Do not commit cookies, generated artifacts, full private URLs, internal responses, or sensitive diagnostics.

See `SECURITY.md`, `PRIVACY.md`, and `CONTRIBUTING.md`.

## Development

```bash
git clone https://github.com/serialq7ic4/ixf-toolbox.git
cd ixf-toolbox
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
```
