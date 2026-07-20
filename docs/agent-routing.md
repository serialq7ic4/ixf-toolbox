# Agent Routing Contract

## Authoritative Current Guidance

Use these files as the current routing source of truth:

- `AGENTS.md`
- `docs/agent-routing.md`
- `skills/*/*/SKILL.md`
- `README.md` and `README.en.md` for user-facing examples

Do not use `docs/superpowers/`, old changelog entries, or historical release
plans to decide current runtime behavior. Those files are implementation
history.

## Natural User Prompts

Users do not need to name skills explicitly. They can paste an authorized
document, sheet, OKR, or Messenger request and describe the desired outcome in
ordinary language. `using-ixf-toolbox` performs background routing to the
correct domain skill.

## Decision Rules

1. Classify the domain: docs, sheets, OKR, or Messenger.
2. Classify the intent: read-only or write.
3. Default ambiguous intent to read-only.
4. For writes, confirm the exact target and content.
5. Use dry-run-first workflows before any remote mutation or message send.
6. Run `ixf doctor --json` when the installed routing or local auth state is unclear; `ixf doctor --json` exposes `agentRouting` for machine-readable verification.

## Runtime Boundary

Go `ixf` is the only supported runtime. Do not call `ixfdoc`, `ixfwrite`,
Python fallback readers, or Python-compatible writers.

## Messenger Boundary

Messenger is local browser automation over a cloned LarkShell profile. It is not
a daemon, bot account, or Open Platform API. Messenger sends are successful only
when `targetVerified:true`, `sent:true`, `localEchoMatched:true`, and
`verifiedPresent:true` are all present.
