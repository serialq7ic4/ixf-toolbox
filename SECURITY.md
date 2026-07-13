# Security

`ixf-toolbox` is a local CLI-first tool for authorized i讯飞/LarkShell document and OKR workflows. It reuses cookies from a user-authorized desktop session and keeps command execution local.

## Safety Defaults

- Reader skills are read-only.
- Writer skills must use dry-run-first workflows.
- Remote write commands default to dry-run.
- Actual mutation requires explicit `--apply`.
- Destructive OKR pruning requires explicit `--prune`.
- Diagnostics and errors must not print secrets or raw private payloads.

## Sensitive Data

Do not share:

- cookie files or cookie values
- CSRF tokens
- full private document or OKR URLs
- account, member, document, Objective, KR, or OKR identifiers
- raw internal API responses
- generated Markdown, TSV, manifest, private Markdown, or OKR content

## Reporting

When reporting an issue, include OS, Python version, `ixf --version`, and redacted command output. Replace private hosts and identifiers with synthetic placeholders.

Never attach cookie files, raw network captures, private screenshots, internal response bodies, or private document content to public issues.
