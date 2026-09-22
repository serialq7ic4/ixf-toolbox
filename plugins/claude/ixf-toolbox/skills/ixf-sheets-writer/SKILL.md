---
name: ixf-sheets-writer
description: Use when writing approved cell values into an authorized i讯飞 sheet or embedded sheet through ixf sheets update, including preparing the tab-separated input file.
---

# ixf Sheets Writer

Use `ixf sheets update` through the local Toolbox CLI. The command writes cells into a live sheet, so use dry-run-first operation and apply only after explicit approval.

Do not route sheet cell writes through `ixf docs update`, which replaces a document body. Sheet reads use `ixf sheets read` or `ixf docs read --expand-sheets` through `ixf-docs-reader`.

## Input Format

`--input` reads **tab-separated values**: one line per row, columns separated by a literal tab. No other format is accepted. JSON and comma-separated files are rejected before any write.

    printf 'evidence text\tassessment text\n' > cells.tsv

Write the file with an explicit tab escape rather than typing spaces, and never hand `--input` a JSON array: `[["a","b"]]` is not two columns, it is one cell containing that text.

`--range` takes an A1-style start cell such as `H3`. A span like `H3:I12` is rejected. The written area is derived from the input shape, so the input file is what decides how many rows and columns are touched.
## Runtime Boundary

Go `ixf` only. Do not call `ixfdoc` or `ixfwrite`. Do not use Python fallback, Python-compatible readers, or Python-compatible writers.

## Runtime Resolution

Resolve the Go `ixf` runtime before running a business command.

1. Prefer `ixf` found on `PATH`.
2. Let `skillFile` be this installed `SKILL.md`. Resolve the plugin root with exactly `filepath.Dir(filepath.Dir(filepath.Dir(skillFile)))`; do not assume the current directory, a repository checkout, a fixed cache path, or a host-specific environment variable. Read `<plugin-root>/runtime.json`.
3. If `ixf` is absent from `PATH`, inspect the documented user-local path: `~/.local/share/ixf-toolbox/bin/ixf` on Unix or `%LOCALAPPDATA%\ixf-toolbox\bin\ixf.exe` on Windows.
4. Run the candidate's `ixf --version` and compare it with `runtime.json.minimumVersion`.
5. If the runtime is missing or too old, select `<plugin-root>/scripts/bootstrap-runtime.sh` or `<plugin-root>/scripts/bootstrap-runtime.ps1`.
6. Run the packaged bootstrap with `--dry-run` before `--apply`. Show the version, platform derived from the asset, release URL host, and target path.
7. Do not download or install anything without explicit user confirmation.
8. After confirmation, run bootstrap `--apply`, invoke the returned installed path directly, then run `ixf doctor --json`.
9. Run `ixf doctor --json` after bootstrap succeeds. Then continue the original request.

Use `ixf deps install`, not bootstrap, for Mermaid dependencies. Never invoke dependency installation silently.

## Workflow

1. Confirm the sheets URL, the start cell, and the exact values to write.
2. Read the target area first, so the start cell is chosen against real row positions rather than assumed ones:
   `ixf sheets read "<sheets-url>"`
3. Write the input file as tab-separated values, one line per row.
4. Run a dry run first:
   `ixf sheets update --url "<sheets-url>" --range H3 --input cells.tsv --dry-run`
5. Check `rows` and `cols` against what you intend to write, and state both to the user. `cols:1` when two columns were intended means the input is not tab-separated; stop and fix the file rather than applying.
6. Apply only after explicit approval:
   `ixf sheets update --url "<sheets-url>" --range H3 --input cells.tsv --apply`
7. Inspect `verify.ok`. Read `verify.scope`: the check confirms the sent values were stored, not that the layout was correct. A wrongly shaped input verifies exactly as faithfully as a correct one, so `verify.ok:true` is not evidence that the intended cells were filled.
8. When layout matters, read the sheet back and compare against intent before reporting success.

## Verification Limits

`verify` compares stored cells against the values that were sent. It cannot detect that the input was shaped wrongly, that the start cell was off by rows, or that a neighbouring column was left empty. Treat it as proof the write landed, not as proof the task was done.

Apply output carries `rangeWarning` when the start row begins below the last populated row and leaves untouched rows between them. That is legitimate when extending a sheet deliberately and a mistake when the start cell is off by rows, and the two are indistinguishable at the API level, so the field names the gap rather than blocking the write. Writing empty strings past the data does not create visible rows.

The dry run cannot produce that warning, because it does not read the sheet; it is local and reports only the shape of the input. So a start cell that may sit past the end of the data must be checked against `ixf sheets read` before applying, per step 2.

## Safety

Write only approved values to a confirmed target. Do not invent cell content. Do not write to ambiguous targets. Do not commit cookies, CSRF tokens, private URLs, workbook or sheet identifiers, private response payloads, or generated private artifacts.

