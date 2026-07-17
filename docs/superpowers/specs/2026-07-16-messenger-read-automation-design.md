# Messenger Read Automation Design

## Goal

Add Go-native, read-only Messenger extraction for recent or unread conversations without exposing message sending.

## Scope

`v3.5.0` adds `ixf messenger read` as an applied browser automation command:

- `--dry-run` validates readiness and prints a plan without launching a browser.
- `--apply` launches a cloned LarkShell profile and reads recent or unread conversations.
- `--scope unread|recent` controls whether only unread cards or the recent list is opened.
- `--limit` controls how many conversations to open.
- `--messages-per-chat` controls how many recent message snippets to return.

Opening chats can mark conversations as read, so browser extraction requires explicit `--apply`.

Real message sending remains out of scope.

## Architecture

The CLI continues to parse flags in `cmd/ixf/main.go`. `internal/messenger` owns planning, profile discovery, safe profile cloning, browser requests, and output shaping.

The existing `Automator` boundary is extended with `Read(context.Context, BrowserReadRequest)`. Tests use a fake automator to verify clone lifecycle, request shaping, and secret-safe payloads. Production uses `chromedp` with DOM extraction based on the existing Messenger selector notes.

## Safety

- Never open the live `profile_explorer` directly.
- Never send messages or touch editor/send controls.
- Keep headless as the default; visible fallback remains explicit.
- Do not print cookies, CSRF tokens, private IDs, profile internals, or screenshots.
- Message text appears only in the requested read result.

## Testing

Use Go tests for read planning, apply lifecycle, CLI argument contracts, selector helpers, repository documentation, and diagnostics. Final verification is `go test ./...`, `go vet ./...`, `git diff --check`, and release binary smoke.
