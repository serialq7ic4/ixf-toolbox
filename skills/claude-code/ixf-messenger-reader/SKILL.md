---
name: ixf-messenger-reader
description: Use when inspecting authorized i讯飞 Messenger readiness, unread-message workflows, chat routing, or local browser-profile prerequisites without sending messages.
---

# ixf Messenger Reader

Use `ixf messenger` through the local Toolbox CLI. This skill is read-only and never sends messages.

## Workflow

1. Confirm the user is asking for Messenger inspection or read-only planning.
2. Check local readiness:
   `ixf messenger doctor --json`
3. If the user wants to open a target, plan it without sending:
   `ixf messenger open --to "<target>" --mode person|conversation --dry-run --json`
4. If the user explicitly accepts that opening a chat may mark it as read, verify the target with:
   `ixf messenger open --to "<target>" --mode person|conversation --apply --json`
5. Treat `targetVerified:true` as the success condition for open verification.

## Safety

Do not print cookie values, CSRF tokens, private conversation IDs, message bodies, screenshots, profile contents, or raw browser state unless the user explicitly needs that content for the requested analysis. `open --apply` never sends messages and must not type into the editor.
