---
name: ixf-okr-writer
description: Use when writing approved Objective and Key Result content into an authorized i讯飞 OKR page after the user has confirmed the exact target.
---

# ixf OKR Writer

Use the `ixf okr` write verbs through the local Toolbox CLI. They are API-only native writers. This skill can modify published OKR content, so use dry-run-first operation and apply only after explicit approval.

`ixf okr write` no longer exists. Each verb below states its own blast radius rather than having it depend on flags and input contents.

| Verb | Does | Destructive |
|---|---|---|
| `ixf okr objective create` | Appends a new Objective with its KRs | No |
| `ixf okr objective retitle` | Rewrites one Objective's title, leaving KRs alone | No |
| `ixf okr kr add` | Appends KRs, keeping the existing ones | No |
| `ixf okr kr replace` | Replaces an Objective's entire KR set | **Yes** |
| `ixf okr kr delete` | Removes named KRs; the only path to zero KRs | **Yes** |
| `ixf okr objective delete` | Deletes one whole Objective and its KRs | **Yes** |

**Never use a destructive verb to accomplish a non-destructive goal.** Adding a KR is `kr add`, not `kr replace` with the existing KRs re-listed. Changing a title is `objective retitle`, not a replace that happens to carry the same KRs.

KR content is passed as repeated `--kr` flags. There is no JSON `--input` file for these verbs.

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

1. Run `ixf okr inspect "<okr-url>"` and show the user the current objectives with their indexes and KR counts. Every write targets an objective by 1-based index, and indexes shift as objectives are added, so never choose one without seeing what is there. Use `inspect` rather than `read`: it reports indexes, identifiers, and counts as JSON, while `read` renders Markdown for a human.
2. Confirm with the user **which verb** matches the intent, using the table above. If the instruction does not clearly select one verb, ask; do not pick the broader one. "Update O3" is ambiguous between `retitle`, `kr add`, and `kr replace`.
3. Run the chosen verb with `--dry-run`, passing `--objective N` and `--expect-title` copied verbatim from step 1. `--expect-title` is required for every verb that targets an existing objective: it is compared before anything is written, so a shifted index refuses instead of writing to the wrong objective.
4. Gate on the payload before proposing apply:
   - `target.titleMatchesExpectation` must be true. If the command refused on a title mismatch, **stop** — the index does not point where you think.
   - `destructive` tells you which class of operation this is. State it to the user in those terms.
   - For destructive verbs, read `diff.krsToDelete` and `diff.krsToDeleteTexts`, and show the user the full list of KR texts that will be destroyed.
   - `diff.resultingKrCount` is what the objective will hold afterwards. **If it is 0 and `diff.krsToDelete` is greater than 0, stop** unless the user explicitly asked to empty the objective.
   - `apply.blocked` states whether apply would be refused and `apply.requiredFlags` names what would satisfy it. Do not attempt apply while it is true.
5. For a destructive verb, get approval for the **number of KRs being destroyed**, not merely for the new content. Name the count and the texts.
6. Apply only after that approval, adding `--apply` and, for destructive verbs, `--confirm-kr-deletes N` with N copied from `diff.krsToDelete`. Never guess N: if it does not match the live count the command refuses, which is the point — a mismatch means the page changed since step 1, so re-run the dry run.
7. Inspect `verify.ok` and read `verify.scope`. The check compares the stored state against what was intended, and `verify.comparedAgainst` names that. It does not confirm other objectives were untouched.
8. Re-read the page with `ixf okr read` or `ixf okr inspect` and confirm only the intended objective changed.

## Why the verbs are separate

The previous single `write` command decided what it did from a flag value and the contents of an input file, and one of those decisions depended on a remote objective count the caller could not see: the same `--objective-index N` replaced an objective's KRs or created a new objective depending on how many already existed. A reviewer reading the command line could not tell whether it would create, edit, or destroy, so approving the command meant approving an unknown.

Each verb now carries one blast radius in its name. `create` refuses any index that is not the append position, so it cannot silently become a replacement. `retitle` never touches KRs and its verification asserts the count did not change. `add` keeps the existing KRs and reports any KR already present in `diff.alreadyPresent` rather than duplicating it.

`kr replace` is one write, not a delete followed by an add. Splitting it would make "objective with no KRs" a state reachable between two commands, which is what made the original defect destroy data rather than merely write the wrong thing. Replacement KRs are created before the old ones are removed, so a failure leaves the originals in place.

`kr delete` is the only way to leave an objective with no KRs, and it takes no input file: destructive intent belongs in the verb and its confirmation, never in data whose shape could be a mistake. A `--kr` that matches nothing aborts the whole command instead of deleting the ones that did match, because a mismatch means the page changed since it was inspected.

`objective delete` replaces the old `--prune`, which removed every objective absent from an input file — a deletion whose extent came from what the data happened to omit. One named objective per invocation makes the extent something the caller states.

## Confirming a destructive write

Destructive verbs require `--confirm-kr-deletes N` matching the number of KRs the write will actually remove. `--apply` alone is not enough, because it is a constant: it can be passed habitually without reading anything. The count cannot, since its correct value exists only in the target's current state, so supplying it is evidence the diff was read.

Take N from `diff.krsToDelete` in the dry run. If it does not match at apply time the command refuses and says the page may have changed — treat that as a signal to re-inspect, not as an obstacle to work around by trying another number.

## Safety

Do not modify O/KR content from vague instructions. Do not use a destructive verb unless removal was explicitly requested. Never pass `--confirm-kr-deletes` with a value you did not read from a dry-run payload in this session; a value carried over from earlier, or copied from an example, can name a count that no longer matches the page.

Do not commit cookies, CSRF tokens, private URLs, person IDs, OKR IDs, or private API payloads.
