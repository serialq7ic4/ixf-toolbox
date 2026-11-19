# macOS Cookie 来源选择与认证诊断设计

日期：2026-11-19

状态：已确认，待实现

## 背景

当前 `ixf cookies export --provider auto` 在 macOS 上只选择最新的
`aha/users/*/profile_explorer/Cookies`。新版 i讯飞客户端可能把文档 Web 会话的
`_csrf_token` 放在 `profile_explorer/Default/Cookies` 或
`PartitionsV2/openplatform_web_container_*/Cookies`。固定路径缺少该 token 时，CLI
会笼统提示桌面客户端未登录，agent 又可能通过不完整的进程名搜索错误判断客户端未运行。

本机复现中，固定路径只有 `swp_csrf_token`，而受支持的 openplatform partition
包含 `_csrf_token`；使用后者导出的 cookie 可以成功读取同一 Claude session 中的目标
wiki 文档。故障属于 cookie DB 选择错误，不是 skill 未触发、客户端未运行或已证明的
CSRF 协议变更。

## 目标

1. macOS 自动导出从真实桌面 profile 的受支持候选库中选择能产生非空
   `_csrf_token` 的来源。
2. 一个候选库缺少 token 或无法解密时继续检查其他候选库。
3. 成功和失败都返回不含 cookie 值的结构化来源诊断，供 agent 准确解释。
4. skill 明确禁止从 cookie 导出失败推断客户端进程状态或登录状态。
5. 保持显式 `--cookies-db`、Windows provider 和下游 docs API CSRF 语义兼容。

## 非目标

- 不把 `swp_csrf_token` 重命名或替代为 `_csrf_token`。
- 不在 cookie 导出时发网络请求或自动打开文档刷新登录态。
- 不扫描 `profile_explorer-playwright-*` 等克隆 profile。
- 不让 `ixf doctor --json` 修改 cookie、客户端或登录状态。
- 不通过 UI 自动化修复认证。

## 方案

考虑过三种方案：

1. 只修正 skill 和错误提示。改动最小，但文档读取仍然失败。
2. 本地候选库选择加结构化诊断。可以恢复当前场景，且保持 cookie export 为本地操作。
3. 每次导出后联网探测 API。能验证服务端会话，但会把本地导出耦合到租户、目标类型和网络状态。

采用方案 2。方案 1 不能解决根因；方案 3 的副作用和耦合超过本次兼容性修复需要。

## 设计

### 候选范围

自动模式只检查每个真实 `aha/users/<user>/` 下的以下位置：

1. `profile_explorer/Cookies`
2. `profile_explorer/Default/Cookies`
3. `PartitionsV2/openplatform_web_container_*/Cookies`

不递归扫描任意 `Cookies` 文件，避免选择 Playwright 克隆、缓存副本或其他应用容器。
显式传入 `--cookies-db` 时只使用该路径，不回退到其他候选。

### 选择规则

每个自动候选按现有 host filter 读取和解密。读取失败、解密失败或没有非空
`_csrf_token` 时记录安全的拒绝原因并继续。第一个可完整导出且含 token 的候选被选中；
同类候选以最近修改时间优先，来源优先级保持上述稳定顺序。

Chromium expiry 作为诊断元数据和 warning 输出，但本次不据此拒绝候选。本机实测中该字段
已过期而服务器仍接受组合会话，单独把它作为失效判据会再次拒绝可用登录态。

### 结构化输出

成功结果增加：

- `sourceKind`
- `candidateCount`
- `csrfExpired`
- `warnings`（仅在需要时）

失败使用可由 `errors.As` 识别的 typed error。`ixf cookies export --json` 输出稳定的
`subtype`、`reason`、`hint`、`retryable` 和候选摘要。摘要只含来源类型、读取/解密状态、
cookie 数量、是否存在所需 token、是否过期和拒绝原因，不包含 cookie 值、完整文档 URL
或私有 API payload。

提示文案必须说明 cookie 来源不可用不等于客户端未运行。对可重试的缺失 token，建议用户
在桌面客户端中打开目标文档以刷新 Web 会话后重试；不指导 agent 用进程名搜索证明认证状态。

### Agent 指引

canonical `using-ixf-toolbox`、docs reader/writer 和 OKR reader/writer 指引统一要求：

- 先读取结构化 cookie export 错误；
- 不从 `_csrf_token` 缺失推断客户端是否运行；
- 不依赖品牌名、bundle 名或可执行文件名的 `pgrep` 结果判断登录态；
- 仅按 CLI 给出的 remediation 请求用户刷新会话。

插件内 skills 继续由 `cmd/pluginpack` 从 canonical skills 生成并通过 `--check` 防漂移。

## 验证

1. 单元测试先覆盖多候选回退、克隆 profile 排除、显式 DB 不回退、过期 warning 和安全错误。
2. CLI 测试覆盖 JSON subtype/hint，确保不再输出“客户端未登录”的确定性结论。
3. 运行 cookies、CLI 和 plugin package 测试，再运行完整 Go 测试。
4. 使用本机真实 profile 导出临时 cookie，读取已授权目标文档；验证后清空临时文件。

