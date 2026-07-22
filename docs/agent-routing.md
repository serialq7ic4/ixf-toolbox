# Agent Routing Contract

## Authoritative Current Guidance

Use these files as the current routing source of truth:

- `AGENTS.md`
- `docs/agent-routing.md`
- `skills/*/*/SKILL.md`
- `README.md` and `README.en.md` for user-facing examples

Do not use `docs/superpowers/`, old changelog entries, or historical release
plans to decide current runtime behavior. Those files are implementation
history.

## Natural User Prompts

Users do not need to name skills explicitly. They can paste an authorized
document, sheet, OKR, or Messenger request and describe the desired outcome in
ordinary language. `using-ixf-toolbox` performs background routing to the
correct domain skill.

## Decision Rules

1. Classify the domain: docs, sheets, OKR, or Messenger.
2. Classify the intent: read-only or write.
3. Default ambiguous intent to read-only.
4. For writes, confirm the exact target and content.
5. Use dry-run-first workflows before any remote mutation or message send.
6. Run `ixf doctor --json` when the installed routing or local auth state is unclear; `ixf doctor --json` exposes `agentRouting` for machine-readable verification.
7. For docs publish readiness, inspect `cookies.ok` and `capabilities.docsPublish`; do not treat top-level `doctor.ok=false` alone as an auth failure.

## Docs Publish Boundary

When the user asks to publish or整理内容到 i讯飞文档, create the Markdown source
and proceed to `ixf docs publish --dry-run` if a tenant/base URL is available
from the prompt or prior i讯飞 link context. If no destination can be inferred,
ask only for the target base URL or parent location. Do not stop with a
local-only Markdown draft after a publish request unless authentication and
cookie export remediation have been attempted or explicitly blocked.

## Sheets Boundary

Direct sheets link reads use `ixf sheets read <sheets-url>`. Embedded sheets
inside supported docx payloads may still be expanded through
`ixf docs read --expand-sheets` when the user is reading the parent document.

Sheet cell update requests must not use `ixf docs update`. Use
`ixf sheets update --url <sheets-url> --range A1 --input cells.tsv --dry-run`
to plan target token, sheet id, range, row count, and column count without
network mutation. After the user confirms the exact target range and TSV input,
use `ixf sheets update --apply` for API-only cell updates and inspect the
returned `verify.ok` result before claiming success. For embedded sheets, keep
`--url` on the direct sheets link and add `--host-url` for the parent docx/wiki
link so the request carries the required host token.

## Runtime Boundary

Go `ixf` is the only supported runtime. Do not call `ixfdoc`, `ixfwrite`,
Python fallback readers, or Python-compatible writers.

## Messenger Boundary

Messenger is local browser automation over a cloned LarkShell profile. It is not
a daemon, bot account, or Open Platform API. Messenger sends are successful only
when `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and
`verifiedPresent:true` are all present.
