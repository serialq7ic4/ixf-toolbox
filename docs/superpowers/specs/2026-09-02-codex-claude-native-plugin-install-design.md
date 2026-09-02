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
4. 让 plugin、旧 raw skill、二进制和可选依赖的状态可以被清楚诊断。
5. 让 native plugin 成为唯一的新安装入口，同时不破坏用户已经存在的 raw skill 文件。

## 非目标

本设计不：

- 把 ixf 二进制嵌入 plugin。
- 在 plugin 安装或首次触发时静默下载二进制、安装依赖或修改系统 PATH；首次使用时，
  只有在用户明确确认后，bootstrap 才能把匹配平台的二进制写入用户目录。
- 删除用户现有的 raw skills。
- 引入 Python、常驻服务、浏览器扩展、远程服务或 Open Platform 应用。
- 修改 docs、sheets、bitable、OKR 或 Messenger 的业务命令语义。

## 决策

采用双层分工：

| 层 | 职责 |
| --- | --- |
| Codex / Claude Code plugin | 提供 skill、提升请求发现和路由、使用宿主原生安装生命周期，并在首次使用时按确认执行 runtime bootstrap |
| Go ixf 二进制 | 执行本地 cookie、文档、OKR、sheets、bitable、Messenger、依赖检查和更新操作 |

plugin 不携带完整 ixf 二进制，但携带一个不含业务 runtime 的 bootstrap 脚本和
runtime 元数据。skill 先查找已有的 `ixf`；找不到或版本低于最低兼容版本时，必须
先获得用户确认，再由 bootstrap 从 GitHub Release 下载匹配平台的二进制，校验
checksum，并原子写入用户目录。已有二进制的日常升级仍由 `ixf update self` 管理。
Mermaid 等可选业务依赖由 `ixf deps install` 管理。

这个边界使宿主 plugin 生命周期与本机 executable 生命周期可独立诊断和升级，同时提供
接近“安装后可直接使用”的首次体验。bootstrap 不修改系统 PATH；skill 会优先使用
PATH 中的 `ixf`，再查找约定的用户目录，并把实际路径传给后续命令。

完整首次使用流程如下：

~~~text
安装 native plugin
  -> skill 被宿主发现
  -> 查找 ixf 并检查最低兼容版本
  -> 缺失或过旧时请求一次明确确认
  -> bootstrap 下载、校验并安装用户目录中的 ixf
  -> ixf doctor --json 复核基础环境
  -> 继续原始 i讯飞任务
~~~

## 仓库结构

将现有双份技能目录收敛成单一 canonical source，并在发布前生成两个宿主专用插件包：

~~~text
.
├── .agents/
│   └── plugins/marketplace.json
├── .claude-plugin/
│   └── marketplace.json
├── plugins/
│   ├── codex/
│   │   └── ixf-toolbox/
│   │       ├── .codex-plugin/plugin.json
│   │       ├── runtime.json
│   │       ├── scripts/
│   │       │   ├── bootstrap-runtime.sh
│   │       │   └── bootstrap-runtime.ps1
│   │       └── skills/
│   └── claude/
│       └── ixf-toolbox/
│           ├── .claude-plugin/plugin.json
│           ├── hooks/
│           │   ├── hooks.json
│           │   └── session-start
│           ├── runtime.json
│           ├── scripts/
│           │   ├── bootstrap-runtime.sh
│           │   └── bootstrap-runtime.ps1
│           └── skills/
├── skills/                         # canonical source only
│   ├── using-ixf-toolbox/
│   │   └── SKILL.md
│   ├── ixf-docs-reader/
│   │   └── SKILL.md
│   ├── ixf-docs-writer/
│   │   └── SKILL.md
│   ├── ixf-okr-reader/
│   │   └── SKILL.md
│   ├── ixf-okr-writer/
│   │   └── SKILL.md
│   ├── ixf-messenger-reader/
│   │   └── SKILL.md
│   └── ixf-messenger-writer/
│       └── SKILL.md
└── scripts/
    └── build-plugin-packages.sh
~~~

skills 目录是所有宿主共用的唯一内容源。仓库不再维护 `skills/codex` 和
`skills/claude-code` 两份 runtime-specific 副本。`plugins/codex/ixf-toolbox` 和
`plugins/claude/ixf-toolbox` 是由构建脚本从 canonical skills 生成的发布包，不是第二
份人工维护的 source of truth；Go runtime 也不 embed 或复制 skill。旧用户目录中的
raw skill 文件只作为迁移对象保留。

`runtime.json` 只包含公开的 bootstrap 元数据，例如 release repository、支持的平台和
最低兼容版本，不包含 token、用户路径或私有配置。bootstrap 必须使用 HTTPS release
地址、校验 checksum、使用临时文件并原子替换，不修改系统级目录或 PATH。

`scripts/build-plugin-packages.sh` 负责从 canonical skills 生成两个包的 `skills/`、
`runtime.json` 和 bootstrap 脚本，支持 `--check` 校验生成结果与源码一致。生成包需要
提交到仓库，因为 marketplace 从 GitHub 源码读取插件；不使用 symlink，也不允许手工
修改生成包中的 skill 内容。Codex marketplace 的 plugin source 指向
`./plugins/codex/ixf-toolbox`，Claude marketplace 的 plugin source 指向
`./plugins/claude/ixf-toolbox`。

Codex 包的 manifest 显式指向 `./skills/`，且不得包含 `hooks` 字段。当前 Codex
manifest 校验不接受该字段，Codex 包也不得包含 Claude 的 `hooks/` 目录：

~~~json
{
  "name": "ixf-toolbox",
  "version": "3.27.0",
  "description": "Use i讯飞 and LarkShell workflows through the Go ixf runtime.",
  "author": {"name": "serialq7ic4"},
  "skills": "./skills/"
}
~~~

Claude 包使用同一份生成后的 skills 内容，并使用标准的 `.claude-plugin/plugin.json`
和 root `hooks/hooks.json`。Claude Code 的 plugin 根目录发现机制负责加载技能和
SessionStart hook；Claude manifest 使用其支持的 skills 数组格式，不把 Codex 的
manifest 字段复制过来。两个包的 manifest 名称都为 `ixf-toolbox`，marketplace 通过
不同的 source path 选择对应宿主包。

## 安装体验

README 将分成 Codex 和 Claude Code 两个原生 plugin 入口。安装 plugin 时不要求用户
预先安装 ixf；首次执行具体 i讯飞任务时再按确认执行 runtime bootstrap。

### Codex

先安装 native plugin：

~~~bash
codex plugin marketplace add serialq7ic4/ixf-toolbox
codex plugin add ixf-toolbox@ixf-toolbox
~~~

Codex 的 plugin list 应显示 ixf-toolbox。新的 Codex session 应加载该 plugin 的
skills。首次实际使用时，skill 会在缺少 ixf 时请求确认并执行 bootstrap；成功后运行
`ixf doctor --json`，然后继续原始任务。

### Claude Code

先安装 native plugin：

~~~bash
claude plugin marketplace add serialq7ic4/ixf-toolbox
claude plugin install ixf-toolbox@ixf-toolbox --scope user
~~~

Claude Code 的 claude plugin list 应显示 ixf-toolbox。安装或更新后需开始新
session 才能稳定加载新的 skills 与 SessionStart 提示。首次实际使用时，skill 会在
缺少 ixf 时请求确认并执行 bootstrap；成功后运行 `ixf doctor --json`，然后继续原始
任务。

### 首次安装的组合说明

README 可以提供一个按宿主区分的完整首装示例，但应把 plugin 安装和 runtime
bootstrap 写成两个有明确触发条件的阶段，而不是要求用户预先完成两个手工安装步骤：

- plugin 安装后即可被宿主发现，但没有 ixf 时只能完成路由和状态提示。
- 缺少 ixf 时，首次实际任务应给出一次具体的下载、版本、平台和目标目录确认。
- 用户确认后 bootstrap 自动完成二进制安装；网络不可用或平台不支持时，再提供手动
  GitHub Release 安装命令。
- bootstrap 不会静默写系统 PATH，也不会安装 Mermaid/Puppeteer 或改变 Messenger
  登录态。

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
| Codex marketplace | `codex plugin marketplace upgrade ixf-toolbox` |
| Codex plugin | `codex plugin add ixf-toolbox@ixf-toolbox` |
| Claude marketplace | `claude plugin marketplace update ixf-toolbox` |
| Claude plugin | `claude plugin update ixf-toolbox@ixf-toolbox` |
| Go 二进制 | `ixf update check --json` 后由用户确认 `ixf update self --apply --json` |
| 缺失 Go 二进制 | plugin bootstrap；用户确认后下载、校验并安装到用户目录 |
| Mermaid 依赖 | `ixf deps install --apply --json`，且仅在用户确认后执行 |

Codex 侧以 marketplace refresh 后重新 add 作为刷新动作，因为本机 CLI 当前没有单独的
plugin update 子命令。README 应说明插件更新和二进制更新是两条独立链路。

## 已有 raw skill 迁移

3.27.0 起不再提供 `ixf setup skills` 或 `ixf update skills`。skill 的安装、更新和
卸载统一交给 Codex / Claude Code 的 native plugin 生命周期；Go CLI 不再复制或刷新
agent skill。

迁移时不自动删除 `~/.codex/skills` 或 `~/.claude/skills` 下的已有目录，避免误删
用户内容。README 和 doctor 应将这些目录标为 legacy raw skills，并建议用户在确认
native plugin 正常工作后自行清理。旧文件可以继续被宿主读取，但不再有 ixf 提供的
安装、更新或修复保证。

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

Codex 包不使用该 hook，不包含 `hooks/` 目录，manifest 也不包含 `hooks` 字段。

## 诊断设计

ixf doctor --json 在现有 agentRouting 下增加 installation 摘要。目标字段为：

~~~json
{
  "agentRouting": {
    "installation": {
      "legacyRawSkills": {
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

legacyRawSkills 的状态通过已知安装目录精确判断。nativePlugin 仅在能够通过稳定、只读的
宿主本地状态判断时报告 installed 或 not-installed；无法稳定判断时必须报告 unknown，
不得猜测。

duplicateLoadRisk 只有当 legacy raw skills 和 native plugin 都被正向确认时才为 true。
`ixf doctor --json` 必须是只读诊断：不得安装依赖、下载二进制、修改 skill 目录或改变
浏览器/登录态。doctor 不应把 native plugin 的 unknown 当成错误，也不应因为未安装
Mermaid 或 Messenger 环境而把基础 docs / OKR / sheets 能力误判为不可用。

依赖检查和依赖修复明确分离：doctor 只报告 `dependencies.mermaid`、
`dependencies.messenger`、`dependencies.update` 及其 remediation；需要执行可脚本化
修复时，用户显式运行 `ixf deps install --dry-run` 预览，再运行
`ixf deps install --apply` 执行。`--apply` 之外的命令不得安装依赖。

## 错误处理与降级

| 情况 | 行为 |
| --- | --- |
| plugin 已安装但找不到 ixf | skill 先检查 ixf --version，说明需安装 GitHub Release 二进制 |
| ixf 版本落后 | 说明 ixf update check / ixf update self 的明确升级路径，不静默升级 |
| plugin CLI 或 marketplace 不可用 | 说明无法通过该宿主完成原生安装，并提供宿主官方安装命令；ixf 不提供 skill fallback |
| native plugin 与 legacy raw skills 同时存在 | 输出重复加载风险和人工迁移建议，不删除文件 |
| Mermaid / Puppeteer 不完整 | 由 doctor 输出诊断；需要修复时使用 `ixf deps install --dry-run/--apply` |
| 宿主 plugin 状态不能可靠探测 | doctor 输出 unknown 和对应的宿主检查命令 |

所有远程文档、OKR、sheets、bitable 和 Messenger 写入的 dry-run-first、--apply 确认和
回读验证语义保持不变。

## 测试与验证

增加或调整以下自动化约束：

1. `plugins/codex/ixf-toolbox/.codex-plugin/plugin.json` 存在，name 为 ixf-toolbox，
   版本符合 strict semver、与 VERSION 一致、skills 指向 `./skills/`，且不包含
   `hooks` 字段；Codex 包不包含 `hooks/`。
2. `plugins/claude/ixf-toolbox/.claude-plugin/plugin.json` 存在，版本与 VERSION 一致，
   Claude 包包含 `hooks/hooks.json`。
3. .agents/plugins/marketplace.json 与 .claude-plugin/marketplace.json 存在并引用
   对应的 ixf-toolbox 宿主包。
4. canonical skills 目录包含全部七个 skill，两个生成包的 skill 内容与 canonical
   内容一致，且生成包不作为 source of truth。
5. Go runtime 不 embed 或复制 skills；不存在 legacy skill installer，只有
   `scripts/build-plugin-packages.sh` 从 canonical skills 生成插件包。
6. 路由 skill 的 frontmatter 包含 i讯飞、LarkShell、/docx/、/wiki/、/base/ 和
   读写意图的必要触发词。
7. README 与 README.en 分开说明 Codex、Claude Code 的 native plugin 安装路径，以及
   已有 legacy raw skills 的迁移提示；不提供 legacy fallback 安装路径。
8. README 不再把 `ixf setup skills` 或 `ixf setup deps` 作为有效命令，并说明
   `ixf doctor` 只读、`ixf deps install` 负责显式依赖修复。
9. doctor 的 legacy-raw-skill、native-plugin unknown 和 confirmed duplicate-risk
   分支都有单元测试。
10. `ixf doctor --json` 的测试确认无安装副作用；`ixf deps install --dry-run` 不执行
    安装，`--apply` 只执行允许的可脚本化依赖安装。
11. GitHub Release 仍只发布 Go 二进制和 checksum。

实现完成后应执行：

~~~bash
scripts/build-plugin-packages.sh --check
go test ./...
go vet ./...
claude plugin validate --strict plugins/claude/ixf-toolbox
claude plugin validate --strict .claude-plugin/marketplace.json
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
6. 在两个新的 agent session 中验证自然路由、二进制发现、doctor 诊断和 legacy raw
   skill 迁移提示。

验收完成的最低标准：

- claude plugin list 显示 ixf-toolbox。
- codex plugin list 显示 ixf-toolbox。
- 两个宿主在出现 i讯飞 / LarkShell 文档、wiki、docx 或 base 链接时能发现路由 skill。
- 两个宿主不会把普通本地 Markdown 默认路由到 ixf。
- 缺失二进制、可选依赖、重复 legacy raw skills 或不支持 native plugin 的宿主都有
  明确且非破坏性的 remediation；不支持 native plugin 时不自动回退为 CLI skill 安装。
