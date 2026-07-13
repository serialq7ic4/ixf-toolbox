# Migration From Legacy Reader/Writer

`ixf-toolbox` replaces the earlier `ixunfei-docx-reader` and `ixunfei-docx-writer`
projects with one local `ixf` command and four agent skills.

New installs should use Toolbox:

```bash
python -m pip install "ixf-toolbox[crypto] @ https://github.com/serialq7ic4/ixf-toolbox/releases/download/v1.0.0/ixf_toolbox-1.0.0-py3-none-any.whl"
ixf setup skills --runtimes auto --json
ixf --version
ixf doctor --json
```

Use `[windows]` instead of `[crypto]` on Windows.

## Command Mapping

| Legacy command | Toolbox command |
|---|---|
| `ixfdoc read` | `ixf docs read` |
| `ixfdoc outline` | `ixf docs outline` |
| `ixfdoc chunk` | `ixf docs chunk` |
| `ixfdoc cleanup` | `ixf docs cleanup` |
| `ixfdoc inspect` | `ixf docs inspect` |
| `ixfdoc cookies export` | `ixf cookies export` |
| `ixfdoc doctor` | `ixf doctor` |
| `ixfdoc setup skills` | `ixf setup skills` |
| `ixfdoc update check` | `ixf update check` |
| `ixfdoc update skills` | `ixf update skills` |
| `ixfwrite docx publish` | `ixf docs publish` |
| `ixfwrite okr write` | `ixf okr write` |
| `ixfwrite cookies export` | `ixf cookies export` |
| `ixfwrite doctor` | `ixf doctor` |
| `ixfwrite setup skills` | `ixf setup skills` |
| `ixfwrite update check` | `ixf update check` |
| `ixfwrite update skills` | `ixf update skills` |

## Skill Mapping

| Legacy skill | Toolbox skill |
|---|---|
| `ixunfei-docx-reader` | `ixf-docs-reader` |
| `ixunfei-docx-writer` document publishing | `ixf-docs-writer` |
| `ixunfei-docx-reader` OKR reading | `ixf-okr-reader` |
| `ixunfei-docx-writer` OKR writing | `ixf-okr-writer` |

## Compatibility Policy

Toolbox does not install `ixfdoc` or `ixfwrite` compatibility shims. The old
commands remain available only if the legacy packages are still installed.

This is intentional:

- It avoids two command surfaces mutating the same private documents or OKR pages.
- It makes agent skill routing explicit: reader skills stay read-only, writer skills stay dry-run-first.
- It keeps new diagnostics, updates, and cookie export behavior centralized under `ixf`.

For automation, migrate scripts to the mapped `ixf` commands instead of relying on
the legacy command names.

## Safety Notes

Migration does not change the authorization model. Toolbox still reuses the local
desktop session, does not run a hosted service, does not collect telemetry, and
does not print cookie values or raw private response payloads.

Generated Markdown, TSV, manifests, cookie files, private URLs, document IDs, OKR
IDs, and OKR content remain sensitive local artifacts.
