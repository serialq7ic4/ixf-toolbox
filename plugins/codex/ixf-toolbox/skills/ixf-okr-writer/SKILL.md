---
name: ixf-okr-writer
description: Use when writing approved Objective and Key Result content into an authorized i讯飞 OKR page after the user has confirmed the exact target.
---

# ixf OKR Writer

Use `ixf okr write` through the local Toolbox CLI. The command is an API-only native writer. This skill can modify published OKR content, so use dry-run-first operation and apply only after explicit approval.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Confirm the OKR URL, objective index, and exact Objective/KR content.
2. Prepare JSON input locally with only the approved content.
   Shape: `{"objectives":[{"objective":"...","krs":["KR1","KR2","KR3"]}]}`
3. Run dry run first:
   `ixf okr write --url "<okr-url>" --input okr.json --objective-index 3 --dry-run`
4. Review the planned Objective/KR changes with the user.
5. Apply only after explicit approval:
   `ixf okr write --url "<okr-url>" --input okr.json --objective-index 3 --apply`
6. Re-read the OKR page after writing and verify only the intended objective changed.

## Safety

Do not modify O/KR content from vague instructions. Do not delete or prune unless explicitly requested. Do not commit OKR JSON files, cookies, CSRF tokens, private URLs, person IDs, OKR IDs, or private API payloads.
