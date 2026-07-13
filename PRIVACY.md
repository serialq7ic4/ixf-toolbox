# Privacy

`ixf-toolbox` runs locally and has no hosted service or telemetry.

The CLI reads local desktop-session cookies and remote i讯飞/LarkShell data only for user-requested document or OKR workflows. Generated files are written to the directory selected by the user.

## Sensitive Local Data

Treat the following as private:

- exported cookie JSON files
- generated Markdown, TSV, and manifest files
- Markdown selected for publication
- OKR input JSON files
- private tenant URLs and identifiers
- diagnostic logs and remote error output

Cookie export uses local desktop login state on supported macOS and Windows systems. Commands must not print cookie values, CSRF values, or raw private response payloads.

Delete temporary cookie, generated artifact, and input files when no longer needed. Do not commit them, paste them into issues, or share them in prompts or screenshots.

Remote writes are performed only after explicit `--apply`. Users are responsible for confirming destination, content, and authorization before applying changes.
