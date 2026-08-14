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

1. Classify the domain: docs, sheets, bitable, OKR, or Messenger.
2. Classify the intent: read-only or write.
3. Default ambiguous intent to read-only.
4. For writes, confirm the exact target and content.
5. Use dry-run-first workflows before any remote mutation or message send.
6. Run `ixf doctor --json` when the installed routing or local auth state is unclear; `ixf doctor --json` exposes `agentRouting` for machine-readable verification.
7. For docs publish readiness, inspect `cookies.ok`, `capabilities.docsPublish`, and `docs.defaultBaseURL`; do not treat top-level `doctor.ok=false` alone as an auth failure.

## Docs Publish Boundary

When the user asks to publish or整理内容到 i讯飞文档, create the Markdown source
and proceed to `ixf docs publish --dry-run` if a tenant/base URL is available
from the prompt, prior i讯飞 link context, `IXF_DOCS_DEFAULT_BASE_URL`, or
`docs.defaultBaseURL` in the local Toolbox config. If no destination can be
inferred and no default is configured, ask only for the target base URL or
parent location. Do not stop with a local-only Markdown draft after a publish
request unless authentication, default-target discovery, and cookie export
remediation have been attempted or explicitly blocked.

## Docs Write Boundary

Use the smallest docs write surface that matches the request:

- New document: `ixf docs publish <file.md> --base-url <tenant> --dry-run`, or omit `--base-url` when `ixf doctor --json` reports `docs.defaultBaseURL.configured=true`.
- Localized insertion or append under an existing heading: `ixf docs patch insert <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --dry-run`.
- Confirmed replacement of one heading section: `ixf docs patch replace-section <fragment.md> --url <doc-or-wiki-url> --under-heading <heading> --dry-run`.
- Confirmed deletion of one heading section: `ixf docs patch delete-section --url <doc-or-wiki-url> --under-heading <heading> --dry-run`.
- Whole-body replacement: `ixf docs update <file.md> --url <docx-url> --dry-run`.

For docx/wiki read or write work, structure preflight is part of the normal workflow. `ixf docs read --out-dir` writes a safe structure summary into the manifest and a `.structure.json` artifact. Existing-docx write dry-runs include a `structure` object with heading paths, section ranges, duplicate headings, and complex-block risk. Agents should inspect that metadata silently and surface it only when it affects target selection, ambiguity, or write safety.

Localized insert requests include prompts such as "insert this table under
heading", "add this block to section 1.1", or "append this content below this
chapter". Route those to `ixf docs patch insert`; do not use `ixf docs update`
for localized insertion because `docs update` is `replace_body` and will replace
the whole body. Before applying patch insert, show `duplicateCandidate`,
`existingBlocksTouched`, `tableBlockType`, `tableFallbackCount`, and any relevant
`structure` ambiguity or complex-block risk. After apply, require
`verify.ok=true` and `verify.unchangedExistingBlocks=true`.

Section replace/delete requests include prompts such as "replace this section" or "delete this section". Route those to `ixf docs patch replace-section` or `ixf docs patch delete-section`, not `ixf docs update`, when the requested scope is one heading section. These operations are bounded destructive edits: dry-run first, inspect `structure`, show `complexBlockTypes` and `outsideSectionBlocksTouched`, require explicit approval before `--apply`, and require `verify.unchangedOutsideSectionBlocks=true` after apply. Do not use section replace/delete for simple insertion. If the target section contains complex blocks, use `--allow-complex-section-replace` only after explicit destructive approval.

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

## Bitable Boundary

Bitable attachment fields are bitable data, not docx blocks or sheet cells. For
direct bitable, wiki bitable, or docx embedded bitable links, use
`ixf bitable inspect --url <url> --json` for safe metadata and
`ixf bitable attach --url <url> --field <attachment-field> --record-id <id> --file <path> --dry-run --json`
or `--record-match Field=Value` to plan an attachment upload. Do not route
bitable attachment requests to `ixf docs update`, docs patch commands, or
`ixf sheets update`.

`ixf bitable attach --apply` is intentionally unavailable until the bitable
asset upload and record-update API contract is captured. Treat dry-run output as
planning only; no remote mutation is supported yet.

## Runtime Boundary

Go `ixf` is the only supported runtime. Do not call `ixfdoc`, `ixfwrite`,
Python fallback readers, or Python-compatible writers.

## Messenger Boundary

Messenger is local browser automation over a cloned LarkShell profile. It is not
a daemon, bot account, or Open Platform API. Messenger sends are successful only
when `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and
`verifiedPresent:true` are all present.
