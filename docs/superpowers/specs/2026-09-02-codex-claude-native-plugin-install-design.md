# Codex 与 Claude Code 原生插件安装设计

日期：2026-09-02

状态：已确认，待实现

## 背景

当前 ixf-toolbox 通过 ixf setup skills 将七个 raw skill 目录复制到
Codex 的 ~/.codex/skills 或 Claude Code 的 ~/.claude/skills。这是可用的
兼容方案，但不是两个宿主各自的原生插件安装方式。

由此产生了几个实际问题：

1. Claude Code 的 claude plugin list 不会显示 ixf-toolbox。
2. Claude Code 面对 i讯飞、LarkShell、docx、wiki 或 base 链接时，技能发现
   不够可靠。
3. raw skills、宿主插件和 Go 二进制分别处于不同位置，但用户看不出哪个负责
   发现、哪个负责执行、哪个需要更新。
4. Codex 与 Claude Code 的 skills 当前是两份字节相同的副本，长期维护存在
   漂移风险。

## 目标

1. 让 Codex 和 Claude Code 都通过各自的原生 plugin 生命周期安装、列出、
   更新和卸载 ixf-toolbox。
2. 保持 Go ixf 二进制为唯一受支持的执行 runtime。
3. 让 i讯飞 / LarkShell 相关的自然语言请求和链接更容易触发正确的路由 skill。
4. 让 plugin、raw skill、二进制和可选依赖的状态可以被清楚诊断。
5. 保持现有 raw skill 安装命令可用，避免破坏已安装用户。

## 非目标

本设计不：

- 把 ixf 二进制嵌入 plugin。
- 在 plugin 安装时静默写 PATH、下载二进制或安装系统依赖。
- 删除用户现有的 raw skills。
- 引入 Python、常驻服务、浏览器扩展、远程服务或 Open Platform 应用。
- 修改 docs、sheets、bitable、OKR 或 Messenger 的业务命令语义。

## 决策

采用双层分工：

| 层 | 职责 |
| --- | --- |
| Codex / Claude Code plugin | 提供 skill、提升请求发现和路由、使用宿主原生安装生命周期 |
| Go ixf 二进制 | 执行本地 cookie、文档、OKR、sheets、bitable、Messenger、依赖检查和更新操作 |

plugin 只告诉 agent 在什么场景调用 ixf 以及如何安全调用。它不携带、下载或升级
二进制。二进制继续由 GitHub Release、ixf update self 和 ixf setup deps 管理。

这个边界使宿主 plugin 生命周期与本机 executable 生命周期可独立诊断和升级，也避免
plugin 安装期间出现平台、PATH、代理、权限和二进制校验混杂的问题。

## 仓库结构

将现有双份技能目录收敛成单一 canonical source：

~~~text
.
├── .agents/plugins/marketplace.json
├── .claude-plugin/
│   ├── marketplace.json
│   └── plugin.json
├── .codex-plugin/
│   └── plugin.json
├── hooks/
│   ├── hooks.json
│   └── session-start
└── skills/
    ├── using-ixf-toolbox/
    │   └── SKILL.md
    ├── ixf-docs-reader/
    │   └── SKILL.md
    ├── ixf-docs-writer/
    │   └── SKILL.md
    ├── ixf-okr-reader/
    │   └── SKILL.md
    ├── ixf-okr-writer/
    │   └── SKILL.md
    ├── ixf-messenger-reader/
    │   └── SKILL.md
    └── ixf-messenger-writer/
        └── SKILL.md
~~~

skills 目录是所有宿主共用的唯一内容源。现有 Go embed、legacy raw-skill
安装器和仓库测试都改为从该目录读取。

Codex manifest 显式指向 ./skills/，并声明空 hooks 对象：

~~~json
{
  "skills": "./skills/",
  "hooks": {}
}
~~~

空 hooks 对象用于禁止 Codex 意外自动发现 Claude Code 的 hooks/hooks.json。

Claude Code plugin 使用同一个 skills 目录，并使用标准的 .claude-plugin
manifest 和 marketplace manifest。Claude Code 对 plugin 根目录下 skills 的原生
发现机制负责加载技能。

## 安装体验

README 将分成两个独立入口，不再将 ixf setup skills 作为默认安装步骤。

### Codex

用户先安装当前平台的 ixf GitHub Release 二进制并确认 ixf --version，随后使用：

~~~bash
codex plugin marketplace add serialq7ic4/ixf-toolbox
codex plugin add ixf-toolbox@ixf-toolbox
ixf doctor --json
~~~

Codex 的 plugin list 应显示 ixf-toolbox。新的 Codex session 应加载该 plugin 的
skills。

### Claude Code

用户先安装当前平台的 ixf GitHub Release 二进制并确认 ixf --version，随后使用：

~~~bash
claude plugin marketplace add serialq7ic4/ixf-toolbox
claude plugin install ixf-toolbox@ixf-toolbox --scope user
ixf doctor --json
~~~

Claude Code 的 claude plugin list 应显示 ixf-toolbox。安装或更新后需开始新
session 才能稳定加载新的 skills 与 SessionStart 提示。

### 首次安装的组合说明

README 可以提供一个按宿主区分的完整首装示例，但仍把二进制安装和 plugin 安装写成
两个显式步骤。二者缺一不可：

- 缺少 plugin 时，agent 不具备稳定的自动发现和路由上下文。
- 缺少 ixf 时，plugin 只能给出明确 remediation，不能执行云文档操作。

## 更新与版本生命周期

每个发布版本使用同一个 VERSION 值作为：

- Go ixf 二进制版本。
- Codex plugin manifest 版本。
- Claude Code plugin manifest 版本。

两个 marketplace 都只声明 ixf-toolbox 的来源和安装策略；Codex 显示的 plugin
版本以该 plugin manifest 为准。Claude Code marketplace 的可选版本元数据若存在，
必须与 manifest 保持一致。

发布新版本后：

| 对象 | 更新方式 |
| --- | --- |
| Codex marketplace | codex plugin marketplace upgrade ixf-toolbox |
| Codex plugin | codex plugin add ixf-toolbox@ixf-toolbox |
| Claude marketplace | claude plugin marketplace update ixf-toolbox |
| Claude plugin | claude plugin update ixf-toolbox@ixf-toolbox |
| Go 二进制 | ixf update check --json 后由用户确认 ixf update self --apply --json |
| Mermaid 依赖 | ixf setup deps --apply --json，且仅在用户确认后执行 |

Codex 侧以 marketplace refresh 后重新 add 作为刷新动作，因为本机 CLI 当前没有单独的
plugin update 子命令。README 应说明插件更新和二进制更新是两条独立链路。

## Legacy raw-skill 兼容

保留以下命令，但明确标记为兼容 fallback：

~~~bash
ixf setup skills --runtimes codex --json
ixf setup skills --runtimes claude-code --json
ixf setup skills --runtimes auto --json
ixf update skills --runtimes auto --json
~~~

它们用于旧版宿主、受限环境或无法使用 plugin marketplace 的场景。新安装文档不再
推荐它们。

迁移时不自动删除 ~/.codex/skills 或 ~/.claude/skills 下的已有目录，避免误删用户
内容。README 和 doctor 应提示用户在 native plugin 与 raw skills 之间选择一种安装
模型。确认双方都存在时，doctor 报告重复加载风险，但不擅自删除任何一方。

## Claude Code 发现增强

using-ixf-toolbox 的 frontmatter description 必须包含高信号触发语言，使自然语言
请求和 URL 更容易选中该路由 skill。至少覆盖：

- i讯飞文档、讯飞文档、LarkShell。
- /docx/、/wiki/、/base/。
- docx、wiki、sheets、bitable、OKR、Messenger。
- 读取、发布、更新、patch、插入、追加行、上传图片或附件。

skill 正文继续承载完整的 Go-only、dry-run-first、权限确认和路由边界，不在
frontmatter 中复制详细流程。

Claude Code plugin 增加轻量 SessionStart hook。它只注入一句短的路由提示：遇到
i讯飞 / LarkShell 链接或相关读写请求时，优先考虑 ixf-toolbox skills。该 hook：

- 不执行 ixf。
- 不读取 cookie、文档或用户文件。
- 不安装二进制或依赖。
- 不注入完整 SKILL.md。
- 不包含私有 URL、token 或用户数据。

Codex 不使用该 hook，并在 manifest 中保持 hooks: {}。

## 诊断设计

ixf doctor --json 在现有 agentRouting 下增加 installation 摘要。目标字段为：

~~~json
{
  "agentRouting": {
    "installation": {
      "rawSkills": {
        "codex": {"status": "installed|not-installed"},
        "claudeCode": {"status": "installed|not-installed"}
      },
      "nativePlugin": {
        "codex": {"status": "installed|not-installed|unknown"},
        "claudeCode": {"status": "installed|not-installed|unknown"}
      },
      "duplicateLoadRisk": false,
      "remediation": []
    }
  }
}
~~~

rawSkills 的状态通过已知安装目录精确判断。nativePlugin 仅在能够通过稳定、只读的
宿主本地状态判断时报告 installed 或 not-installed；无法稳定判断时必须报告 unknown，
不得猜测。

duplicateLoadRisk 只有当 raw skills 和 native plugin 都被正向确认时才为 true。
doctor 不应把 native plugin 的 unknown 当成错误，也不应因为未安装 Mermaid 或
Messenger 环境而把基础 docs / OKR / sheets 能力误判为不可用。

## 错误处理与降级

| 情况 | 行为 |
| --- | --- |
| plugin 已安装但找不到 ixf | skill 先检查 ixf --version，说明需安装 GitHub Release 二进制 |
| ixf 版本落后 | 说明 ixf update check / ixf update self 的明确升级路径，不静默升级 |
| plugin CLI 或 marketplace 不可用 | 建议显式使用 legacy ixf setup skills |
| native plugin 与 raw skills 同时存在 | 输出重复加载风险和人工迁移建议，不删除文件 |
| Mermaid / Puppeteer 不完整 | 保持现有 doctor 和 ixf setup deps 显式 remediation |
| 宿主 plugin 状态不能可靠探测 | doctor 输出 unknown 和对应的宿主检查命令 |

所有远程文档、OKR、sheets、bitable 和 Messenger 写入的 dry-run-first、--apply 确认和
回读验证语义保持不变。

## 测试与验证

增加或调整以下自动化约束：

1. .codex-plugin/plugin.json 存在，name 为 ixf-toolbox，版本符合 strict semver，
   与 VERSION 一致，skills 指向 ./skills/，hooks 恰为 {}。
2. .claude-plugin/plugin.json 存在，版本与 VERSION 一致。
3. .agents/plugins/marketplace.json 与 .claude-plugin/marketplace.json 存在并引用
   ixf-toolbox。
4. canonical skills 目录包含全部七个 skill，且不再以 runtime-specific 副本作为
   source of truth。
5. Go embed 和 legacy installer 从 canonical skills 读取。
6. 路由 skill 的 frontmatter 包含 i讯飞、LarkShell、/docx/、/wiki/、/base/ 和
   读写意图的必要触发词。
7. README 与 README.en 分开说明 Codex、Claude Code 和 legacy fallback 的安装路径。
8. README 不再将 ixf setup skills 描述为新安装默认步骤。
9. doctor 的 raw-skill、native-plugin unknown 和 confirmed duplicate-risk 分支都有
   单元测试。
10. GitHub Release 仍只发布 Go 二进制和 checksum。

实现完成后应执行：

~~~bash
go test ./...
go vet ./...
claude plugin validate --strict .
~~~

还应在隔离的 plugin 配置环境中，从仓库根目录手动验证：

~~~bash
codex plugin marketplace add .
codex plugin add ixf-toolbox@ixf-toolbox
codex plugin list

claude plugin marketplace add .
claude plugin install ixf-toolbox@ixf-toolbox --scope user
claude plugin list
~~~

验证 native plugin 后必须新开 Codex 和 Claude Code session，确认 i讯飞相关自然语言
请求能发现 using-ixf-toolbox，而普通本地 Markdown 的阅读、审阅和编辑仍使用宿主文件
系统。

## 发布与验收

该工作是新的安装与分发能力，版本从 3.26.3 升至 3.27.0。

发布需要：

1. 更新 VERSION、两个 plugin manifest、marketplace metadata 和 CHANGELOG。
2. 更新 README、README.en、docs/agent-routing.md、docs/go-python-parity.md、
   docs/release.md 与相关 doctor 文档。
3. 通过 Go 测试、vet、plugin manifest 校验和宿主生命周期 smoke test。
4. 创建 v3.27.0 tag，通过 GitHub Release 发布 Go 二进制与 checksum。
5. 发布后更新本机 ixf 二进制，并分别通过 Codex 和 Claude Code 原生 plugin 命令
   安装或刷新 plugin。
6. 在两个新的 agent session 中验证自然路由、二进制发现、doctor 诊断和 legacy
   fallback 提示。

验收完成的最低标准：

- claude plugin list 显示 ixf-toolbox。
- codex plugin list 显示 ixf-toolbox。
- 两个宿主在出现 i讯飞 / LarkShell 文档、wiki、docx 或 base 链接时能发现路由 skill。
- 两个宿主不会把普通本地 Markdown 默认路由到 ixf。
- 缺失二进制、可选依赖、重复 raw skills 或不支持 plugin 的宿主都有明确且非破坏性的
  remediation。
