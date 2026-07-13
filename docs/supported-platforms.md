# Supported Platforms

`ixf-toolbox` targets desktop i讯飞/LarkShell environments with local authenticated session data. The CLI owns cookie export, diagnostics, document workflows, and OKR workflows; agent skills call the CLI.

| Platform | Status | Notes |
|---|---|---|
| macOS | Tier 1 | `ixf cookies export --provider auto` reads the local Chromium profile and decrypts cookies with Keychain. |
| Windows | CI-tested / experimental | `ixf cookies export --provider windows-larkshell` reads the local Chromium profile and decrypts cookies with DPAPI through `pywin32`; more live desktop validation is required. |

Linux desktop-session export is not supported because i讯飞 does not ship a Linux desktop client. Pure parsing and dry-run components may still work, but authenticated remote operations require a supported cookie source.

## Windows

Install with:

```powershell
python -m pip install "ixf-toolbox[windows]"
ixf cookies export --provider windows-larkshell --output %TEMP%\ixf_cookies.json
ixf doctor --json --cookies %TEMP%\ixf_cookies.json
```

Exported cookie files are sensitive. Do not log, screenshot, commit, or retain them longer than needed.
