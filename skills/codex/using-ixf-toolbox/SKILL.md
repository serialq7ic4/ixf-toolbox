---
name: using-ixf-toolbox
description: Use when an i讯飞/LarkShell document, wiki, docx, sheets, cloud document, OKR, or messenger request needs routing to the correct ixf Toolbox reader or writer skill.
---

# Using ixf Toolbox

Use this as a lightweight routing skill for ixf Toolbox workflows. Users do not need to name this skill or any domain skill explicitly. Use background routing for natural user requests, then hand off to the correct domain skill. This skill does not replace the domain skills and does not perform direct writes.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Routing

- Use `docs/agent-routing.md`, `AGENTS.md`, and current `skills/*/*/SKILL.md` files as authoritative current guidance.
- Do not route from historical implementation notes, old changelog entries, or `docs/superpowers/` plans.
- Use `ixf-docs-reader` for authorized document, wiki, docx, cloud-doc, direct sheets link, embedded sheet, mindnote, image artifact, or local Markdown reading.
- Use `ixf-docs-writer` for approved Markdown publishing or document modification.
- Use `ixf-okr-reader` for authorized OKR reading, summary, ownership, mention, or alignment analysis.
- Use `ixf-okr-writer` for approved Objective and Key Result creation or modification.
- Use `ixf-messenger-reader` for authorized i讯飞 Messenger readiness checks and read-only message inspection workflows.
- Use `ixf-messenger-writer` for approved Messenger sends after dry-run planning and explicit apply confirmation.

## Decision Rules

1. Classify the request as docs, OKR, or messenger.
2. Classify the intent as read or write.
3. Default ambiguous intent to read-only. Default to read-only when uncertain.
4. For writes, confirm the exact target and content, then follow the relevant writer skill's dry-run-first workflow.
5. If local authentication or installed routing looks unclear, run `ixf doctor --json` and inspect `agentRouting`.
6. If local authentication looks missing, run or suggest `ixf cookies export --provider auto`.

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, document IDs, OKR IDs, person IDs, or generated private artifacts unless the user explicitly needs that content for the requested analysis.
