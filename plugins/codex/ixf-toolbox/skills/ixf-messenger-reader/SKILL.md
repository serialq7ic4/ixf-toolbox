---
name: ixf-messenger-reader
description: Use when inspecting authorized i讯飞 Messenger readiness, unread or recent messages, chat routing, or local browser-profile prerequisites without sending messages.
---

# ixf Messenger Reader

Use `ixf messenger` through the local Toolbox CLI. This skill is read-only and never sends messages. Messenger automation is Chrome/Chromium-only and always runs against a cloned profile, never the live LarkShell profile.

## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Workflow

1. Confirm the user is asking for Messenger inspection or read-only message analysis.
2. Check local readiness:
   `ixf messenger doctor --json`
3. If the user wants unread or recent message content, start with a read plan:
   `ixf messenger read --scope unread|recent --dry-run --json`
4. If the user explicitly accepts that reading may mark opened chats as read, read through the cloned profile:
   `ixf messenger read --scope unread|recent --apply --json`
5. If the user wants to open a target, plan it without sending:
   `ixf messenger open --to "<target>" --mode person|conversation --dry-run --json`
6. If the user explicitly accepts that opening a chat may mark it as read, verify the target with:
   `ixf messenger open --to "<target>" --mode person|conversation --apply --json`
7. Treat `targetVerified:true` as the success condition for open verification.

## Safety

Do not print cookie values, CSRF tokens, private conversation IDs, screenshots, profile contents, or raw browser state. Message bodies may appear only when the user requested message reading or analysis. `read --apply` and `open --apply` never send messages and must not type into the editor.
