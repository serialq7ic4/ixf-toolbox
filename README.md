# ixf-toolbox

`ixf-toolbox` provides one local `ixf` command and four agent skills for authorized i讯飞 document and OKR workflows.

Toolbox is migrating implementation into this repository in stages while keeping the user-facing `ixf` commands stable. Some document and OKR workflows still delegate to the legacy reader/writer engines; cookie/session export and diagnostics are now Toolbox-owned.

## Install

```bash
python -m pip install "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v0.3.0/ixf_toolbox-0.3.0-py3-none-any.whl"
ixf setup skills --runtimes auto --json
ixf --version
```

On Windows, use the `windows` extra:

```bash
python -m pip install "ixf-toolbox[windows] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v0.3.0/ixf_toolbox-0.3.0-py3-none-any.whl"
```

## Commands

| Command | Purpose |
| --- | --- |
| `ixf docs read <source>...` | Read authorized cloud document links or local Markdown files |
| `ixf docs publish <file.md> ...` | Publish Markdown as an authorized cloud document |
| `ixf okr read <url>` | Read an authorized OKR page |
| `ixf okr write --url <url> --input <file.json>` | Write confirmed OKR content |
| `ixf cookies export ...` | Export local desktop session cookies |
| `ixf doctor --json` | Inspect Toolbox prerequisites without printing cookie values |
| `ixf setup skills --runtimes auto --json` | Install Codex and Claude Code skill wrappers |
| `ixf update check` | Check the latest GitHub release |
| `ixf update skills --runtimes auto --json` | Refresh installed skill wrappers |

## Examples

Read a document:

```bash
ixf docs read "https://tenant.example.test/wiki/example" --out-dir /tmp/ixf-read --print-manifest
```

Publish Markdown with a dry run:

```bash
ixf docs publish notes/review.md --base-url https://tenant.example.test --dry-run
```

Write approved OKR content:

```bash
ixf okr write --url "https://tenant.example.test/okr/user/example" --input okr.json --dry-run
```

Check local setup:

```bash
ixf doctor --json
```

## Skills

`ixf setup skills` installs these wrappers:

- `ixf-docs-reader`
- `ixf-docs-writer`
- `ixf-okr-reader`
- `ixf-okr-writer`

Reader skills are read-only. Writer skills require confirmed content and should run dry-run first before real writes.

## Migration

The existing projects stay available during migration:

- `ixunfei-docx-reader`
- `ixunfei-docx-writer`

New workflows should prefer `ixf`. Existing workflows using `ixfdoc` or `ixfwrite` can continue until Toolbox reaches full implementation parity.

## Security

Do not commit cookies, CSRF tokens, passwords, real tenant URLs, private document IDs, OKR IDs, private response payloads, or generated private artifacts unless explicitly approved.
