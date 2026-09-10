# Agent Guidance

`ixf-toolbox` is a Go-only runtime repository. All current document, wiki, docx,
sheets, bitable, OKR, cookie, dependency, update, and Messenger work must use
the local Go `ixf` CLI.

- Do not use Python fallback, Python-compatible readers, or Python-compatible writers.
- Do not call `ixfdoc` or `ixfwrite`; those are legacy commands from archived projects.
- Do not infer current routing from old `CHANGELOG.md` entries or `docs/superpowers/` plans.
  Those files are historical implementation records.
- Use `README.md`, `docs/go-python-parity.md`, and `skills/*/SKILL.md` as the
  current operating guidance.
- Codex and Claude native plugin commands own skill installation and lifecycle.
  `ixf doctor --json` is read-only; `ixf deps install --apply` is the only
  optional dependency mutation path.
