# Migration From Legacy Reader/Writer

`ixf-toolbox` replaces the archived `ixunfei-docx-reader` and
`ixunfei-docx-writer` projects with one Go `ixf` runtime and seven agent skills.
New installations use a native plugin for each host:

```bash
codex plugin marketplace add serialq7ic4/ixf-toolbox
codex plugin add ixf-toolbox@ixf-toolbox
```

```bash
claude plugin marketplace add serialq7ic4/ixf-toolbox
claude plugin install ixf-toolbox@ixf-toolbox --scope user --yes
```

The native plugin owns skill discovery and lifecycle. On first use it resolves
the Go runtime or presents a bootstrap dry-run; installation requires explicit
confirmation, verifies the release checksum, stays in the user directory, and
does not modify `PATH`.

## Migration Procedure

1. Install the native plugin for the active host.
2. Start a new Codex or Claude session.
3. Verify that a natural i讯飞 document or Messenger request discovers the plugin.
4. Run `ixf doctor --json` and inspect native plugin state and legacy duplicate risk.
5. After routing is verified, manually remove obsolete raw skill directories if desired.

Existing `~/.codex/skills/ixf-*`, `~/.claude/skills/ixf-*`, and
`using-ixf-toolbox` directories are legacy installations. Native plugins do not
delete them: **do not delete automatically** any legacy skill directory. The
user owns the final cleanup decision.

## Command Mapping

| Legacy command | Current Go command |
|---|---|
| `ixfdoc read` | `ixf docs read` |
| `ixfdoc outline` | `ixf docs outline` |
| `ixfdoc chunk` | `ixf docs chunk` |
| `ixfdoc cleanup` | `ixf docs cleanup` |
| `ixfdoc inspect` | `ixf docs inspect` |
| `ixfdoc cookies export` | `ixf cookies export` |
| `ixfdoc doctor` | `ixf doctor` |
| `ixfdoc update check` | `ixf update check` |
| `ixfwrite docx publish` | `ixf docs publish` |
| `ixfwrite okr write` | `ixf okr write` |
| `ixfwrite cookies export` | `ixf cookies export` |
| `ixfwrite doctor` | `ixf doctor` |
| `ixfwrite update check` | `ixf update check` |

## Skill Mapping

| Legacy use | Current skill |
|---|---|
| Broad i讯飞 workflow requests | `using-ixf-toolbox` |
| Document, wiki, sheet, and bitable reads | `ixf-docs-reader` |
| Document publishing and approved updates | `ixf-docs-writer` |
| OKR reading | `ixf-okr-reader` |
| Approved OKR writes | `ixf-okr-writer` |
| Messenger readiness and conversation reads | `ixf-messenger-reader` |
| Approved Messenger sends | `ixf-messenger-writer` |

## Removed commands

These command surfaces were removed when skill installation moved to native
plugins and optional dependency repair became a separate confirmed operation:

| Removed command | Replacement |
|---|---|
| `ixf setup skills` | Codex or Claude native plugin installation and lifecycle commands |
| `ixf setup deps` | `ixf deps install --dry-run --json`, then confirmed `--apply` |
| `ixf update skills` | Codex or Claude native plugin update commands |

## Compatibility Policy

Toolbox does not install `ixfdoc` or `ixfwrite` compatibility shims. Migrate
automation to the mapped Go commands instead of keeping two command surfaces
that can mutate the same private documents or OKR pages.

Python package installation is not supported. The repository and release
artifacts are Go-only; historical Python references remain only as deletion
evidence.

## Safety Notes

Migration does not change authorization. Toolbox reuses the local desktop
session, runs locally, does not collect telemetry, and does not print cookie
values or raw private response payloads. Generated Markdown, TSV, manifests,
cookie files, private URLs, document IDs, OKR IDs, and OKR content remain
sensitive local artifacts.
