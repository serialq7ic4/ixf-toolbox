# Messenger Send Automation Design

## Goal

Add Go-native Messenger sending with explicit user approval, target verification before typing, and fresh-session verification after sending.

## Scope

`v3.6.0` adds `ixf messenger send`:

- `--dry-run` validates the target, mode, message length, and readiness without launching a browser or sending.
- `--apply` sends a real message only after opening and verifying the target chat.
- Fresh-session verification is mandatory: after the send session closes, Toolbox opens a second cloned profile and verifies that the sent text is present as a self message.
- Supported target modes are `person` and `conversation`.

Unread/recent reads remain read-only. Sending is the only Messenger mutation in this release.

## Architecture

The CLI parses `messenger send` flags in `cmd/ixf/main.go`. `internal/messenger` owns planning, profile cloning, browser requests, and safe payload shaping.

`Automator` gains `Send(context.Context, BrowserSendRequest)`. `SendMessage` creates two profile clones: one for the send session and one for fresh verification. Production `ChromedpAutomator.Send` uses the existing open/verify actions, then writes to the chat editor, triggers send, waits for a local echo, closes, opens the second profile, and confirms the self message is present.

## Safety

- `--apply` is required for a real send.
- Dry-run must not launch a browser.
- Do not send unless target verification succeeds.
- Do not report success unless fresh-session verification succeeds.
- Do not print cookie values, CSRF tokens, private IDs, screenshots, profile internals, or raw DOM state.
- Do not echo full message bodies in command results; return length and verification booleans instead.

## Testing

Use Go tests for send planning, dry-run no automation, apply clone lifecycle, fresh verification clone usage, message redaction, CLI argument contracts, skill documentation, diagnostics capabilities, and release smoke.
