---
name: ixf-messenger-writer
description: Use when sending an approved i讯飞 Messenger message to a confirmed person or conversation through local Toolbox automation.
---

# ixf Messenger Writer

Use `ixf messenger send` through the local Toolbox CLI. This skill can send real messages only after dry-run planning and explicit user confirmation. Messenger automation is Chrome/Chromium-only and always runs against cloned profiles.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Confirm the exact recipient or conversation, target mode, and message text.
2. Run readiness checks:
   `ixf messenger doctor --json`
3. Run dry-run send planning:
   `ixf messenger send --to "<target>" --mode person|conversation --message "<text>" --dry-run --json`
4. Show the dry-run plan and ask for explicit approval before applying.
5. If approved, send with:
   `ixf messenger send --to "<target>" --mode person|conversation --message "<text>" --apply --json`
6. Treat `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and `verifiedPresent:true` as the success condition. The command performs fresh-session verification before reporting success.

## Safety

Never send on ambiguous intent. Do not type into Messenger manually or fall back to another script. Do not use `open --apply` as a substitute for sending. Command output should not echo full message bodies; rely on message length and verification booleans.
