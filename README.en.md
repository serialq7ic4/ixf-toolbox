# ixf-toolbox

`ixf-toolbox` provides one local `ixf` command and four agent skills for authorized i讯飞/LarkShell document and OKR workflows.

The Toolbox command owns document reading and publishing, OKR reading and writing, cookie/session export, diagnostics, updates, and agent skill installation.

## Install

```bash
python -m pip install "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v1.0.0/ixf_toolbox-1.0.0-py3-none-any.whl"
ixf setup skills --runtimes auto --json
ixf --version
ixf doctor --json
```

On Windows, use the `windows` extra.

## Commands

| Command | Purpose |
|---|---|
| `ixf docs read <source>...` | Read authorized cloud document links or local Markdown files |
| `ixf docs publish <file.md> ...` | Publish Markdown as an authorized cloud document |
| `ixf okr read <url>` | Read an authorized OKR page |
| `ixf okr write --url <url> --input <file.json>` | Write confirmed OKR content |
| `ixf cookies export ...` | Export local desktop session cookies |
| `ixf doctor --json` | Inspect local setup without printing cookie values |
| `ixf setup skills --runtimes auto --json` | Install Codex and Claude Code skill wrappers |
| `ixf update self --json` | Plan or apply a package upgrade |

Reader skills are read-only. Writer skills require confirmed content and should run dry-run first before real writes.

## Safety

Do not commit cookies, CSRF tokens, private URLs, private document IDs, OKR IDs, private response payloads, generated artifacts, or private content.

See `SECURITY.md`, `PRIVACY.md`, and `CONTRIBUTING.md`.
