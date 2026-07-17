# Supported Platforms

`ixf-toolbox` targets desktop i讯飞/LarkShell environments with local authenticated session data. The Go binary is the default local runtime. The CLI owns cookie export, diagnostics, document workflows, OKR workflows, and staged Messenger automation foundations; agent skills call the CLI.

| Platform | Status | Notes |
|---|---|---|
| macOS | Tier 1 | `ixf cookies export --provider auto` reads the local Chromium profile and decrypts cookies with Keychain. `ixf messenger doctor` discovers `profile_explorer` under the LarkShell app support directory and requires profile cloning before browser automation. |
| Windows | CI-tested / experimental | `ixf cookies export --provider windows-larkshell` reads the local Chromium profile and decrypts cookies with Windows DPAPI. Messenger profile discovery uses the LarkShell Chromium profile under `%APPDATA%`; more live desktop validation is required. |

Linux desktop-session export and Messenger automation are not supported because i讯飞 does not ship a Linux desktop client. Pure parsing and dry-run document/OKR components may still work, but authenticated remote operations require a supported cookie source.

## Messenger

Messenger automation is browser-profile-first, not cookie-only. The live LarkShell profile must never be opened directly; Toolbox clones the profile into a temporary directory, removes Chromium singleton locks and cache-heavy directories, and only then allows later browser automation stages to run against the clone.

`v3.5.0` exposes diagnostics, dry-run open planning, explicit --apply target verification, and read-only conversation extraction:

```bash
ixf messenger doctor --json
ixf messenger open --to "示例群聊" --mode conversation --dry-run --json
ixf messenger open --to "示例群聊" --mode conversation --apply --json
ixf messenger read --scope unread --dry-run --json
ixf messenger read --scope unread --apply --json
```

Opening or reading chats may mark them as read. Real message sending remains intentionally unavailable.

## Windows

Install the Go binary with:

```powershell
New-Item -ItemType Directory -Force $HOME\bin | Out-Null
Invoke-WebRequest -Uri https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.5.0/ixf_3.5.0_windows_amd64.exe -OutFile $HOME\bin\ixf.exe
$env:PATH = "$HOME\bin;$env:PATH"
ixf cookies export --provider windows-larkshell --output $env:TEMP\ixf_cookies.json
ixf doctor --json --cookies $env:TEMP\ixf_cookies.json
```

Exported cookie files are sensitive. Do not log, screenshot, commit, or retain them longer than needed.
