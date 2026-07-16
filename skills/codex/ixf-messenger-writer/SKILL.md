---
name: ixf-messenger-writer
description: Use when planning an approved i讯飞 Messenger send target or message, while real sending is not yet available in the current Toolbox release.
---

# ixf Messenger Writer

Use `ixf messenger open` through the local Toolbox CLI for dry-run send planning only.

## Workflow

1. Confirm the exact recipient or conversation, target mode, and message text.
2. Run readiness checks:
   `ixf messenger doctor --json`
3. Run dry-run target planning:
   `ixf messenger open --to "<target>" --mode person|conversation --dry-run --json`
4. Report that Real sends are not available in this release.

## Safety

Real sends are not available. Do not simulate success, type into a chat manually, or fall back to another messenger script. Future send support must verify the opened target before typing and re-open a fresh session after send.
