# Messenger Runbook

This runbook covers the GA operation boundary for `ixf messenger`. Messenger support is local-browser automation over the user's already-authorized i讯飞/LarkShell desktop session. It is not a daemon, bot account, or Open Platform integration.

## Prerequisites

- macOS or Windows with the i讯飞/LarkShell desktop client installed and signed in.
- Google Chrome or Chromium installed. Messenger browser discovery is Chrome/Chromium-only by default.
- A readable LarkShell `profile_explorer` directory. `ixf messenger doctor --json` reports the discovered path.
- A local cookie JSON file from `ixf cookies export`; diagnostics report only metadata, never cookie values.

Run readiness checks first:

```bash
ixf messenger doctor --json
```

When prerequisites are missing, `doctor` returns `ok:false` and a `remediation` list. The guidance is intentionally generic and secret-safe.

## Profile Isolation

Messenger automation must never open the live LarkShell browser profile directly. Toolbox copies `profile_explorer` to a temporary cloned profile, removes Chromium singleton locks and cache-heavy directories, and runs Chrome/Chromium against the clone.

The clone is deleted after the command by default. Use `--keep-profile-clone` only for local debugging, and treat the retained profile as sensitive.

## Read-Only Operations

Use dry-run first:

```bash
ixf messenger open --to "示例群聊" --mode conversation --dry-run --json
ixf messenger read --scope unread --dry-run --json
```

Apply only after accepting the side effect:

```bash
ixf messenger open --to "示例群聊" --mode conversation --apply --json
ixf messenger read --scope unread --apply --json
```

`read/open may mark chats as read` because the cloned browser opens real Messenger conversations through the authorized session. These commands never send messages and must not type into the editor.

Open verification succeeds only when `targetVerified:true` is returned.

## Sending

Sending is a confirmed write operation. Always plan first:

```bash
ixf messenger send --to "示例群聊" --mode conversation --message "示例消息" --dry-run --json
```

Dry-run does not launch a browser and does not echo the full message body. After explicit user approval, apply:

```bash
ixf messenger send --to "示例群聊" --mode conversation --message "示例消息" --apply --json
```

A successful send requires all of these booleans:

- `targetVerified:true`
- `sent:true`
- `localEchoMatched:true`
- `verifiedPresent:true`

The command first verifies the target, sends through a cloned profile, checks the local echo, then uses a second fresh cloned profile to verify that the message is present in the conversation.

## Browser Behavior

Headless mode is the default. `--allow-visible-fallback` may open a visible browser only when explicitly requested for troubleshooting. Chrome/Chromium-only discovery intentionally excludes Edge to keep the runtime predictable across macOS and Windows.

Use one of these overrides for local debugging:

```bash
IXF_MESSENGER_BROWSER_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  ixf messenger doctor --json

ixf messenger doctor --browser-path "/path/to/chrome" --json
```

## Troubleshooting

| Symptom | Action |
|---|---|
| `browser ok=false` | Install Google Chrome or Chromium, or pass `--browser-path` / set `IXF_MESSENGER_BROWSER_PATH`. |
| `profile ok=false` | Open i讯飞/LarkShell desktop, sign in, then rerun; pass `--profile-dir` if auto discovery is wrong. |
| `cookies ok=false` | Run `ixf cookies export --provider auto --output /tmp/ixunfei_profile_explorer_cookies.json`. |
| Messenger opens login page | Refresh cookies and confirm the desktop client is still signed in. |
| Target is not verified | Retry with a more exact person account id or conversation title. Do not send unless target verification succeeds. |

## Safety Rules

- Do not print cookies, CSRF tokens, raw DOM, screenshots, private conversation IDs, or retained profile contents.
- Do not send on ambiguous user intent.
- Do not use `open --apply` as a substitute for `send --apply`.
- Do not report send success unless `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and `verifiedPresent:true` are all present.
