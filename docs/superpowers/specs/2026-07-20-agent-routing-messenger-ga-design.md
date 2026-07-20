# Agent Routing And Messenger GA Hardening Design

## Goal

Ship `v3.8.0` as a small hardening release that makes current agent routing
authoritative and makes Messenger readiness boundaries visible in diagnostics.

## Scope

- Keep `ixf-toolbox` Go-only. Do not reintroduce Python runtime, Python scripts,
  Python tests, `ixfdoc`, or `ixfwrite`.
- Treat `docs/superpowers/` and old changelog entries as historical records, not
  current routing guidance.
- Improve `using-ixf-toolbox` so users can describe work naturally and the skill
  routes in the background.
- Add machine-readable diagnostics that expose the current routing contract and
  Messenger GA boundary.
- Keep Messenger as local browser automation over a cloned LarkShell profile,
  not a daemon, bot account, or Open Platform API.

## Architecture

The root `ixf doctor` payload will include an `agentRouting` object. It records
that routing is Go-only, background-routed through `using-ixf-toolbox`, defaults
ambiguous intent to read-only, and ignores historical implementation docs for
current execution decisions.

`ixf messenger doctor` will include a `stability` object under `messenger`. It
records the operating model, platform support level, and send success criteria,
so agents can distinguish "usable local automation" from a hosted messaging
service.

The skill wrappers remain the primary agent UX. `using-ixf-toolbox` should
explain that users do not need to name skills explicitly. Domain reader/writer
skills keep dry-run-first and explicit `--apply` boundaries.

## Testing

- Repository contract tests require the new routing doc and skill guidance.
- CLI tests require `ixf doctor --json` to expose `agentRouting`.
- Messenger doctor tests require `stability` metadata and no duplicate
  remediation entries.
- Full verification remains `go test ./...`, `go vet ./...`, `git diff --check`,
  and release workflow success.
