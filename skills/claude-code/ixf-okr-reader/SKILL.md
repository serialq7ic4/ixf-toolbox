---
name: ixf-okr-reader
description: Use when reading authorized i讯飞 OKR pages to summarize objectives, key results, mentions, ownership, alignment, or planning inputs.
---

# ixf OKR Reader

Use `ixf okr read` through the local Toolbox CLI. This skill is read-only.

## Workflow

1. Accept only OKR links the user is authorized to access.
2. Export cookies first if the local session is missing:
   `ixf cookies export --provider auto`
3. Read the OKR page:
   `ixf okr read "<okr-url>"`
4. Summarize Objective/KR content, mentions, and alignment points.
5. Keep any copied output local unless the user asks to persist it.

## Safety

Do not print cookie values, CSRF tokens, private API payloads, full private URLs, person IDs, OKR IDs, or sensitive personnel information unless necessary for the user's requested analysis.
