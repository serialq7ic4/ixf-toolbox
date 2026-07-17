# Agent Guidance

`ixf-toolbox` is a Go-only runtime repository. All current document, wiki, docx,
sheets, OKR, cookie, update, setup, and Messenger work must use the local Go
`ixf` CLI.

- Do not use Python fallback, Python-compatible readers, or Python-compatible writers.
- Do not call `ixfdoc` or `ixfwrite`; those are legacy commands from archived projects.
- Do not infer current routing from old `CHANGELOG.md` entries or `docs/superpowers/` plans.
  Those files are historical implementation records.
- Use `README.md`, `docs/go-python-parity.md`, and `skills/*/*/SKILL.md` as the
  current operating guidance.
