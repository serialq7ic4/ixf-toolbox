# Messenger Open Automation Design

## Goal

Ship `v3.4.0` with Go-native browser automation that can open a Messenger person or conversation and verify the visible target, without exposing message sending.

## Scope

This stage upgrades `ixf messenger open` from dry-run-only planning to an explicitly applied read-side browser action:

- `--dry-run` keeps the existing no-browser planning behavior.
- `--apply` clones the LarkShell profile, launches a controlled Chromium-family browser, opens Messenger, searches or selects the target, and verifies the visible chat title or panel text.
- Missing both `--dry-run` and `--apply` is invalid.
- Real message sending, unread extraction, and group mentions remain out of scope.

Opening a chat can mark messages as read, so `--apply` is required even though no message is sent.

## Architecture

`internal/messenger` gains a small browser automation boundary:

- `OpenTarget(config, automator)` owns validation, profile discovery, profile clone lifecycle, and result shaping.
- `Automator` is an interface so tests can verify clone/cleanup and payload behavior without launching a browser.
- `ChromedpAutomator` is the default production implementation using `chromedp` against the cloned `profile_explorer`.

The CLI remains thin: parse flags, call `OpenTarget`, and print JSON or text.

## Safety

- The live LarkShell profile is never opened directly.
- Browser launch uses the cloned profile path only.
- Headless remains default. Visible fallback requires `--allow-visible-fallback`.
- `open --apply` returns `targetVerified:true` only when the opened title or panel text matches the requested target.
- The command never types into the message editor and never clicks send.
- Diagnostics and errors must not print cookie values, CSRF tokens, private IDs, or message bodies.

## Testing

Tests cover apply gating, fake-automator invocation, clone cleanup, keep-clone behavior, title matching, CLI flag parsing, and repository documentation. Live browser behavior is verified by compile/build and kept behind explicit `--apply` for local manual validation.
