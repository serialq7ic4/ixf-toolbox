# Supported Platforms

`ixf-toolbox` targets desktop i讯飞/LarkShell environments with local authenticated session data. Native Codex and Claude plugins provide the skills; the Go binary is the only business runtime. The CLI owns cookie export, diagnostics, document workflows, OKR workflows, dependency repair, runtime updates, and staged Messenger automation foundations.

| Platform | Status | Notes |
|---|---|---|
| macOS | Tier 1 | `ixf cookies export --provider auto` reads the local Chromium profile and decrypts cookies with Keychain. `ixf messenger doctor` discovers `profile_explorer` under the LarkShell app support directory and requires cloned profile isolation before Chrome/Chromium browser automation. |
| Windows | CI-tested / experimental | `ixf cookies export --provider windows-larkshell` reads the local Chromium profile and decrypts cookies with Windows DPAPI. Messenger profile discovery uses the LarkShell Chromium profile under `%APPDATA%`; Messenger automation still uses a cloned profile and needs more live desktop validation. |

Linux desktop-session export and Messenger automation are not supported because i讯飞 does not ship a Linux desktop client. Pure parsing and dry-run document/OKR components may still work, but authenticated remote operations require a supported cookie source.

## Plugin Bootstrap Paths

On first use, a native plugin checks `PATH` and then the user-local runtime path.
If the runtime is missing or too old, the packaged bootstrap shows a dry-run and
requires explicit confirmation before downloading and checksum verification. It
does not modify `PATH`.

| Platform | User-local runtime path |
|---|---|
| macOS / Linux | `~/.local/share/ixf-toolbox/bin/ixf` |
| Windows | `%LOCALAPPDATA%\ixf-toolbox\bin\ixf.exe` |

Manual GitHub Release binary installation remains a troubleshooting fallback
when bootstrap cannot reach the release host. Plugin lifecycle remains owned by
the Codex or Claude plugin command, while `ixf update check/self` owns runtime
updates and `ixf deps install --apply` owns confirmed optional Mermaid dependency
repair.

## Messenger

Messenger automation is browser-profile-first, not cookie-only. The live LarkShell profile must never be opened directly; Toolbox clones the profile into a temporary directory, removes Chromium singleton locks and cache-heavy directories, and only then allows later browser automation stages to run against the cloned profile.

Messenger browser automation auto-discovers Chrome/Chromium only. Install Google Chrome on macOS or Windows, or pass an explicit `--browser-path` when using another Chromium-compatible browser for local debugging.

`v3.8.0` exposes diagnostics with remediation, dry-run open planning, explicit --apply target verification, read-only conversation extraction, approved sends with fresh-session verification, and machine-readable stability metadata:

```bash
ixf messenger doctor --json
ixf messenger open --to "示例群聊" --mode conversation --dry-run --json
ixf messenger open --to "示例群聊" --mode conversation --apply --json
ixf messenger read --scope unread --dry-run --json
ixf messenger read --scope unread --apply --json
ixf messenger send --to "示例群聊" --mode conversation --message "示例消息" --dry-run --json
ixf messenger send --to "示例群聊" --mode conversation --message "示例消息" --apply --json
```

Opening or reading chats may mark them as read. Sending requires explicit `--apply`, target verification, local echo matching, and fresh-session verification before success is reported.

See [`docs/messenger.md`](messenger.md) for the full Chrome/Chromium-only Messenger runbook.

## Windows

After plugin bootstrap or manual runtime installation, export and diagnose the
local Windows session with:

```powershell
& "$env:LOCALAPPDATA\ixf-toolbox\bin\ixf.exe" cookies export --provider windows-larkshell --output $env:TEMP\ixf_cookies.json
& "$env:LOCALAPPDATA\ixf-toolbox\bin\ixf.exe" doctor --json --cookies $env:TEMP\ixf_cookies.json
```

Exported cookie files are sensitive. Do not log, screenshot, commit, or retain them longer than needed.
