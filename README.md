# ixf-toolbox

**简体中文** | [English](README.en.md)

让 Codex、Claude Code 等本地 coding agent 读取、发布已授权访问的 i讯飞/LarkShell 云文档，读取或写入经过确认的 OKR 内容，并接入本机 Messenger 工作流。

> 面向本地 agent 使用，`ixf` 是统一执行入口；复用本机登录态，无服务端，无遥测，不需要飞书开放平台应用。

<p>
  <img alt="go" src="https://img.shields.io/badge/Go-1.24%2B-00ADD8">
  <img alt="platform" src="https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20experimental-lightgrey">
  <img alt="license" src="https://img.shields.io/badge/license-Apache%202.0-green">
</p>

`ixf-toolbox` 提供一个本地 CLI 和七个 agent skill：

- `using-ixf-toolbox`: 轻量路由入口，在文档/OKR/Messenger、读取/写入意图不明确时选择正确的具体 skill。
- `ixf-docs-reader`: 只读，读取已授权云文档、本地 Markdown、动态分块和图片产物。
- `ixf-docs-writer`: 写入，先 dry-run，再将 Markdown 发布为新 docx 文档；`publish` 不覆盖已有 docx。
- `ixf-okr-reader`: 只读，读取已授权 OKR 页面并输出 Objective / Key Result Markdown。
- `ixf-okr-writer`: 写入，先 dry-run，再创建或修改经过确认的 Objective / Key Result。
- `ixf-messenger-reader`: 只读，检查 Messenger 自动化就绪状态，读取最近或未读会话。
- `ixf-messenger-writer`: 写入，先 dry-run，再在目标验证和 fresh-session 复核后发送确认消息。

项目刻意保持本地化和窄边界。它不是浏览器扩展、常驻 daemon、同步服务、批量迁移工具，也不替代组织的数据权限和审批流程。

## 为什么做这个

私有 i讯飞/LarkShell 文档、OKR 页面和桌面端 Messenger 通常不能被 coding agent 通过普通 HTTP fetch 直接读取或修改。`ixf-toolbox` 补的是这段本地工作流：

- agent 通过 Codex / Claude Code skill 调用本机 `ixf`。
- `ixf` 复用你已经登录的桌面端会话。
- 读取类能力把授权内容转换为本地 Markdown/TSV/manifest，便于 agent 分析。
- 写入类能力默认只生成 dry-run 计划，确认后才用 `--apply` 发起远程写入。
- cookie、诊断、生成产物和输入文件默认都留在本机。

和面向浏览器的一键导出工具相比，Toolbox 的入口更偏向 agent 工作流：

| 项目形态 | 更适合 |
|---|---|
| Codex / Claude Code skill + `ixf` | 在本地研发、产品、运维等日常工作中让 agent 处理已授权文档和 OKR |
| 浏览器扩展 | 浏览器内一键导出、可视化 UI、PDF/HTML、附件批量下载等工作流 |

## 安装到 Codex / Claude Code

推荐让当前正在使用的 agent 直接完成安装。默认安装方式是下载 GitHub Release Go 二进制，再把七个 skill 注册到 Codex 或 Claude Code；不要求本机具备 Python 环境。

如果你正在使用 Codex，可以直接对 Codex 说：

> 请帮我安装 https://github.com/serialq7ic4/ixf-toolbox。使用 GitHub Release Go 二进制安装本地 `ixf`（macOS Apple Silicon 用 `ixf_3.11.2_darwin_arm64`，macOS Intel 用 `ixf_3.11.2_darwin_amd64`，Windows 用 `ixf_3.11.2_windows_amd64.exe`），然后运行 `ixf setup skills --runtimes codex --json` 注册 skill，最后用 `ixf --version` 和 `ixf doctor --json` 验证。

### macOS Apple Silicon

```bash
mkdir -p ~/.local/bin
curl -L -o ~/.local/bin/ixf \
  https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.11.2/ixf_3.11.2_darwin_arm64
chmod +x ~/.local/bin/ixf
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

macOS Intel 将文件名换成 `ixf_3.11.2_darwin_amd64`。

### Windows PowerShell

```powershell
New-Item -ItemType Directory -Force $HOME\bin | Out-Null
Invoke-WebRequest -Uri https://github.com/serialq7ic4/ixf-toolbox/releases/download/v3.11.2/ixf_3.11.2_windows_amd64.exe -OutFile $HOME\bin\ixf.exe
$env:PATH = "$HOME\bin;$env:PATH"
ixf setup skills --runtimes codex --json
ixf --version
ixf doctor --json
```

### 同时安装到两个 agent

将上面的 `--runtimes codex` 换成 `--runtimes auto`，即可同时注册 Codex 和 Claude Code skill。

### Go-only runtime

v3.1 起仓库已删除 Python runtime/package 和 Python 测试 harness，只保留 Go `ixf` 作为受支持的执行入口。开发、CI 和发布验证统一使用 Go 工具链。

当前版本所有文档、wiki、docx、sheets、OKR、cookie、setup、update 和 Messenger 能力都只走 Go `ixf`。不要使用 Python fallback，不要调用旧的 `ixfdoc` 或 `ixfwrite` 命令；历史 changelog 和 `docs/superpowers/` 计划文件不能作为当前路由依据。当前路由契约见 [`docs/agent-routing.md`](docs/agent-routing.md)，`ixf doctor --json` 会输出 `agentRouting` 诊断。

## 在 Agent 里使用

安装 skill 后，可以直接让 agent 处理授权链接、本地文件或 Messenger 请求。你不需要点名具体 skill；直接按日常方式描述目标即可。using-ixf-toolbox 会在后台识别链接类型、读写意图和安全边界，再分配给对应的文档、OKR 或 Messenger skill。

> 帮我总结一下这个文档：https://tenant.example.test/wiki/example

> 看一下这个 OKR 页面，梳理和我相关的 Objective / Key Result。

> 将 `notes/review.md` 发布到 `https://tenant.example.test`，先展示 dry-run 计划，我确认后再实际写入。

> 把我确认后的 O3 和 3 个 KR 写入这个 OKR 页面，只修改 O3，先展示 dry-run 计划。

> 看一下未读消息，帮我汇总需要我处理的事项。

> 给这个群发一段消息，先展示 dry-run 计划，等我确认后再继续。

第一次读取或写入私有远程内容前，需要先确保本机 i讯飞/LarkShell 桌面端已登录。Toolbox 会通过本机登录态导出 cookie 并复用授权会话。

## 底层命令

通常不需要手动调用这些命令；它们主要用于调试、自动化或排查登录态问题。

| 命令 | 用途 |
|---|---|
| `ixf docs read <source>...` | 将授权云文档链接或本地 Markdown 读取为 Markdown/TSV/manifest 产物 |
| `ixf docs outline <file.md>` | 按标题和原子块生成动态阅读目录 |
| `ixf docs chunk <file.md> --index <n>` | 输出指定动态分块 |
| `ixf docs inspect <source>` | 输出安全路由摘要，不读取正文、不打印完整 token |
| `ixf docs cleanup <out-dir>` | 删除读取流程生成的文件和图片产物 |
| `ixf docs publish <file.md>` | 将 Markdown 发布为新的授权 docx 文档，不覆盖已有 docx |
| `ixf docs update <file.md> --url <docx-url> --dry-run` | 规划替换已有 docx 正文，不执行写入 |
| `ixf docs update <file.md> --url <docx-url> --apply` | 替换已有 docx 正文，默认拒绝复杂块；确认后可加 `--allow-complex-replace` |
| `ixf okr read <url>` | 读取授权 OKR 页面并输出 Markdown |
| `ixf okr write --url <url> --input <file.json>` | 创建或修改确认后的 Objective / KR |
| `ixf messenger doctor --json` | 检查 Messenger 自动化所需的桌面端 profile、浏览器和 cookie 元数据 |
| `ixf messenger open --to <target> --mode person\|conversation --dry-run --json` | 规划打开联系人或会话，不发送消息 |
| `ixf messenger open --to <target> --mode person\|conversation --apply --json` | 启动克隆 profile 的浏览器并验证目标会话，不发送消息 |
| `ixf messenger read --scope unread\|recent --dry-run --json` | 规划读取未读或最近会话，不启动浏览器 |
| `ixf messenger read --scope unread\|recent --apply --json` | 启动克隆 profile 的浏览器并读取会话片段，不发送消息 |
| `ixf messenger send --to <target> --mode person\|conversation --message <text> --dry-run --json` | 规划发送消息，不启动浏览器、不回显完整消息正文 |
| `ixf messenger send --to <target> --mode person\|conversation --message <text> --apply --json` | 发送确认消息，并通过 fresh-session 复核后报告成功 |
| `ixf cookies export` | 从本机桌面端会话导出 cookie |
| `ixf doctor --json` | 检查运行环境、skill 和 cookie 元数据，不打印 cookie 值 |
| `ixf setup skills --runtimes auto --json` | 安装 Codex / Claude Code skill |
| `ixf update check --json` | 检查最新 GitHub Release |
| `ixf update self --json` | 规划或执行 Toolbox 自升级 |
| `ixf update skills --runtimes auto --json` | 刷新本地 skill wrapper |

### Runtime 状态

v2.4 起 Go 二进制拥有已文档化的 CLI runtime：文档读取/发布、OKR 读取/写入、cookie export、doctor、skill setup 和 update flow。v2.6 起 GitHub Release 只发布 Go 二进制和 checksum；v3.0 起 Python runtime/package 实现已删除；v3.1 起测试和发布流程也不再依赖 Python；v3.3 起 Messenger 进入 Go-native 分阶段上线；v3.4 起支持显式 --apply 打开并验证目标会话；v3.5 起支持只读读取未读或最近会话；v3.6 起支持确认后的消息发送，并要求 fresh-session 复核；v3.7 起补齐 Messenger GA 运行手册和可执行诊断提示；v3.8 起补齐 agent routing 诊断和 Messenger 稳定边界元数据；v3.9 起支持已有 docx 正文替换的 dry-run/preflight；v3.10 起支持确认后的已有 docx 正文替换写入；v3.11 起补齐复杂块显式覆盖开关和更新 runbook。

Agent 路由契约见 [`docs/agent-routing.md`](docs/agent-routing.md)。Messenger 详细运行手册见 [`docs/messenger.md`](docs/messenger.md)，覆盖 Chrome/Chromium-only discovery、cloned profile 隔离、读取副作用和发送成功判定。

## 更新

Toolbox 不做静默自动更新。推荐先检查最新 GitHub Release：

```bash
ixf update check --json
```

查看 dry-run 计划：

```bash
ixf update self --json
```

确认后执行升级，并刷新本地 skill：

```bash
ixf update self --apply --json
```

如果只想刷新本地 skill wrapper：

```bash
ixf update skills --runtimes auto --json
```

## 手动读取流程

如果需要绕过 agent skill 做底层调试，可以手动执行：

1. 打开 i讯飞/LarkShell 桌面端，并确认已经登录。
2. 导出本地会话 cookie。
3. 用 `doctor` 检查 cookie 文件形态，不会打印 cookie 值。
4. 可选用 `inspect` 检查单个来源的安全路由摘要，不读取正文。
5. 读取一个或多个授权文档链接。

```bash
ixf cookies export \
  --provider auto \
  --output /tmp/ixf_cookies.json

ixf doctor \
  --json \
  --cookies /tmp/ixf_cookies.json

ixf docs inspect \
  "https://tenant.example.test/wiki/example" \
  --json

ixf docs read \
  "https://tenant.example.test/wiki/example" \
  "https://tenant.example.test/sheets/example?sheet=sheet1" \
  --cookies /tmp/ixf_cookies.json \
  --out-dir ./out \
  --expand-sheets \
  --download-images \
  --print-manifest
```

生成的 Markdown、TSV、图片和 manifest 都是本地文件。如果源文档敏感，这些产物也应按敏感数据处理。

常用读取参数：

| 参数 | 用途 |
|---|---|
| `--out-dir <dir>` | 生成产物目录 |
| `--cookies <file>` | `ixf cookies export` 导出的 cookie JSON 文件 |
| `--expand-sheets` | 将支持的 docx 嵌入 sheet 展开为 TSV；直接 sheets 链接会默认读取为 TSV |
| `--download-images` | 下载可认证访问的 docx 图片块到本地 assets 目录 |
| `--print-manifest` | 输出 JSON manifest，包含产物路径和元数据 |
| `--cleanup` | 命令退出前删除本次命令生成的文件 |

`--cleanup` 只会删除本次命令生成的文件，不会递归删除输出目录里的其他内容。

## 手动写入流程

写入类命令默认 dry-run。实际远程写入必须显式传入 `--apply`。

### 发布 Markdown 为新 docx

```bash
ixf docs publish notes/review.md \
  --base-url https://tenant.example.test
```

确认标题、block 统计和目标租户后实际新建发布：

```bash
ixf docs publish notes/review.md \
  --base-url https://tenant.example.test \
  --cookies /tmp/ixf_cookies.json \
  --apply
```

### 更新已有 docx

`ixf docs update` 使用 `replace_body` 模式：保留原 URL、权限和位置，替换正文 blocks。默认拒绝包含图片、内嵌表格等复杂块的原文档；详见 [`docs/docs-update.md`](docs/docs-update.md)。

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --dry-run
```

确认 dry-run 计划后执行：

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --require "关键内容" \
  --apply
```

如果 dry-run 报告复杂块，只有在明确接受丢失这些复杂块后才使用覆盖开关：

```bash
ixf docs update notes/review.md \
  --url https://tenant.example.test/docx/example \
  --cookies /tmp/ixf_cookies.json \
  --allow-complex-replace \
  --apply
```

### 写入 OKR

输入文件示例：

```json
[
  {
    "objective": "提升核心服务稳定性与交付效率",
    "krs": [
      "完成关键链路风险治理并形成复盘机制",
      "降低高频故障的恢复时间",
      "完善容量与变更检查流程"
    ]
  }
]
```

只修改 O3，默认 dry-run：

```bash
ixf okr write \
  --url "https://tenant.example.test/okr/user/example/?okrId=example" \
  --input okr.json \
  --cookies /tmp/ixf_cookies.json \
  --objective-index 3
```

确认计划后实际写入：

```bash
ixf okr write \
  --url "https://tenant.example.test/okr/user/example/?okrId=example" \
  --input okr.json \
  --cookies /tmp/ixf_cookies.json \
  --objective-index 3 \
  --apply
```

`--objective-index` 用于只修改指定 Objective；当目标序号等于当前 Objective 数量 + 1 时会创建新的 Objective，并验证其他 Objective 未被改变。不传 `--objective-index` 时，Go 运行时会按 Objective 文本匹配并写入多个 Objective。`--prune` 会删除输入中没有保留的内容，仅在明确需要时使用。

## 支持的能力

当前 Toolbox 覆盖：

- i讯飞/LarkShell `docx` 文档读取与 Markdown 转换。
- 可解析到受支持文档类型的 `wiki` 链接读取，包括 docx token 解析和 bitable TSV 输出。
- 直接 mindnote / sheets 链接读取，以及通过受支持文档 payload 暴露出来的 mindnote 标记和嵌入 sheet TSV 展开。
- 简单表格、任务列表、代码块语言、富文本链接、图片块下载、直接 sheets 读取、嵌入 sheet 展开和安全资源清理。
- 本地 Markdown 分块、读取、发布和测试。
- 授权 OKR 页面读取、指定 Objective 更新/创建、按 Objective 文本写入多个 Objective、KR 创建/修改/排序、显式 prune 和发布。
- Messenger 自动化就绪诊断、profile 发现、profile 安全克隆（cloned profile）、dry-run 打开规划、显式 --apply 打开并验证目标会话、只读读取最近/未读会话，以及确认后的消息发送和 fresh-session 复核。
- 本机 macOS / Windows 桌面端 cookie 导出、诊断和 skill 安装。

部分云文档 block 格式无法和 Markdown 一一对应。当前转换器优先保证 agent 分析可用，而不是完全还原原始文档视觉效果。

## 支持平台

| 平台 | 状态 | 说明 |
|---|---|---|
| macOS | Tier 1 | 读取 LarkShell Chromium profile，并通过 Keychain 解密 cookie；Messenger 诊断会发现并克隆 `profile_explorer`；Messenger 浏览器自动发现只使用 Chrome/Chromium。 |
| Windows | CI-tested / experimental | 读取 LarkShell Chromium profile，并通过 DPAPI 解密 cookie；Messenger profile 发现仍需要更多真实桌面端验证；Messenger 浏览器自动发现只使用 Chrome/Chromium。 |

Linux 不支持桌面会话导出，因为 i讯飞没有 Linux 桌面客户端。纯解析和 dry-run 组件仍可在具备依赖的环境中使用。

更多细节见 [`docs/supported-platforms.md`](docs/supported-platforms.md) 和 [`docs/messenger.md`](docs/messenger.md)。

## 迁移

旧的 reader / writer 项目已经归档。新安装和后续功能统一使用 `ixf-toolbox`。

迁移命令映射见 [`docs/migration-from-legacy.md`](docs/migration-from-legacy.md)。

## 项目维护

- 安全和隐私：[`SECURITY.md`](SECURITY.md)、[`PRIVACY.md`](PRIVACY.md)
- 贡献规范：[`CONTRIBUTING.md`](CONTRIBUTING.md)
- 发布流程：[`docs/release.md`](docs/release.md)
- 平台状态：[`docs/supported-platforms.md`](docs/supported-platforms.md)
- CI / Release：`.github/workflows/`

## 隐私与安全

- Cookie 导出复用本机桌面端登录态。
- `doctor` 不会打印 cookie 值。
- 远程读取错误不会回显原始 API payload。
- 远程写入默认 dry-run，必须显式使用 `--apply`。
- Messenger 当前支持诊断、dry-run 打开规划、显式 --apply 目标验证、只读会话读取，以及确认后的消息发送；发送成功必须通过 fresh-session 复核。Messenger 自动化只自动发现 Chrome/Chromium，并始终使用 cloned profile。
- 删除 OKR 多余内容需要额外显式使用 `--prune`。
- 生成的 Markdown、TSV、图片、manifest 和 OKR JSON 可能包含私有内容。
- 不要提交 cookie、生成产物、完整私有链接、内部响应或带敏感元数据的诊断输出。
- 本工具仅用于读取或写入你已获授权访问的内容。请遵守所在组织的数据管理要求。

## 开发

```bash
git clone https://github.com/serialq7ic4/ixf-toolbox.git
cd ixf-toolbox
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(cat VERSION)" -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
```
