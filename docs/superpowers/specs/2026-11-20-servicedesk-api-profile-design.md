# Agentic ServiceDesk API 与动态 Profile Overlay 设计

日期：2026-11-20

状态：已审批

## 背景

`ixf-toolbox` 当前覆盖 i讯飞文档、Wiki、Sheets、Bitable、OKR 和 Messenger，
但团队提交 ServiceDesk 工单时仍依赖网页操作或针对单个表单硬编码请求。

现有 Autobot 的服务器硬件报修实现证明了 ServiceDesk 支持 API Key 认证和 API
写入，但它把 `snapshot_id`、顶层字段 key 和嵌套表格列 key 固定在 Python 代码中。
同一工单历史上已经多次变更 `snapshot_id`，因此把完整 payload 内置进 Go 二进制或
静态 profile 都会产生明显的版本漂移风险。

2026-11-20 使用以下测试工单完成了协议验证：

- 表单 URL：`https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003`
- `businessId`：`15`
- `service_id`：`4003`
- 当时的 `snapshot_id`：`218527`
- API Key-only 元数据读取、创建、详情查询和撤回均成功
- 测试工单：`REQ20261120052741`，内部 ID `3463015`
- 创建后状态为 `RUNNING`，撤回后状态为 `WITHDRAWN`

验证同时确认：ServiceDesk 网页虽然没有公开表单元数据文档，但前端使用的内部 API
可以通过个人 API Key 直接读取当前服务和完整表单定义。因此不再需要依赖浏览器 F12、
HAR 重放或 UI 自动化来维护工单字段。

## 意图

为团队打造可供本地 Agent 调用的 ServiceDesk 工单能力。用户用自然语言告诉 Agent 要创建、
查询或撤回什么工单；Agent 获取当前表单的机器可读契约，将用户意图整理为结构化输入，执行
dry-run，在写操作前取得明确确认，再完成 API 写入和回读验证。

Go-native、API-first 的 `ixf servicedesk` CLI 是这套 Agent 能力的确定性执行底座，而不是最终
交互入口。它不内置大模型或自行解释自然语言，只负责实时表单发现、结构化校验、安全写入和
机器可读结果。Codex、Claude Code 等原生 plugin/skill 负责自动发现和调用；任何能够执行本地
CLI 并解析 JSON 的其他 Agent 也可以使用同一契约接入。

安装 `ixf-toolbox` 后，用户不需要知道 `ixf servicedesk` 命令、profile 名或具体 skill 名。用户
只需对 Agent 说“我要提个工单”“查一下这个工单”或“撤回这个工单”，路由 skill 就应识别意图，
选择 ServiceDesk 读取或写操作 skill 并在后台调用 CLI；仅在目标工单类型、必填业务事实或写入
确认缺失时向用户提问。

每位用户使用自己的 API Key，CLI 直接调用 ServiceDesk；团队只共享少量业务语义 overlay，
不共享凭据，也不通过中央代理代发工单。

## 用户使用场景

### 查看支持的工单

用户问“目前可以提哪些工单”时，Agent 读取 Registry profile 列表，用业务名称和适用范围回答，
而不是展示 `serviceId`、snapshot 或后端字段 key。用户继续选择某一类时，Agent 再读取实时表单
契约。该场景不创建工单，也不要求 ServiceDesk API Key。

### 基于当前上下文提单

用户可以说“帮我提单报修上面这个问题”或“提单报修下这个硬盘”。Agent 应使用当前会话中已经
明确出现的服务器、设备 SN、硬盘、故障现象和计划时间等事实，不要求用户机械重复；然后选择
合适 profile，将事实映射为结构化 input。上下文仍缺少必填业务事实时再精准询问，不能推测或
编造。dry-run 后必须向用户展示目标工单类型和内容摘要并取得确认。

### 按业务描述撤回工单

用户可以说“撤回 xx 硬盘的报修工单”或“把刚才那个报修单撤了”，不要求提供 ticket ID。Agent
先读取当前用户近期、尚可撤回的工单，再结合当前会话和工单摘要语义定位目标。只有一个明确
目标时进入 withdraw dry-run；多个合理目标时展示候选并询问。最终撤回仍需本次明确确认。

### 查看我的近期或未完成工单

用户可以问“我最近提了哪几个工单”或“看看当前还没处理完的工单”。Agent 返回当前 API Key
用户本人创建的工单，包含工单号、标题、类型、状态、当前步骤/处理人、创建时间和详情 URL。
“未处理完”不能只按标题猜测，必须根据服务端状态分类；未知状态不得被静默排除。

首个版本至少支持：

1. Agent 列出当前 Registry 支持的工单类型和适用范围。
2. Agent 从工单 URL 或共享 profile 获取当前、版本化的输入契约。
3. Agent 使用当前会话事实构造 input，并通过 dry-run 获得可操作的缺失字段或漂移信息。
4. 用户确认准确目标和内容后，Agent 使用显式 `--apply` 创建工单并验证结果。
5. Agent 列出当前用户近期或未完成的工单，并查询单张工单的当前流转状态。
6. Agent 根据用户的业务描述从本人近期工单中定位撤回目标；用户确认后执行撤回并回读验证为
   `WITHDRAWN`。

## 目标

1. 对 Agent 提供稳定、版本化、机器可读的工单输入契约、执行结果和错误分类。
2. 安装 Toolbox 后，用户通过明确的工单能力查询或动作意图即可触发能力，无需点名 Toolbox、
   skill 或 CLI；单纯描述故障、设备或业务问题不得触发提单流程。
3. ServiceDesk 实时元数据是表单结构的唯一事实来源。
4. `/disk1/files` 仅保存轻量业务 overlay，团队客户端通过
   `http://10.103.251.43/files/` 下载。
5. API Key 只存在于用户本机和发往 ServiceDesk 的请求中，不进入 Agent prompt、overlay、日志或
   dry-run 输出。
6. 写操作保持仓库现有的 dry-run、用户明确确认、显式 `--apply` 和写后回读验证契约。
7. 服务端表单变化时自动检测并分类；非破坏性变化可继续，破坏性变化失败关闭。
8. Agent 接入层不绑定单一宿主；Codex/Claude plugin 提供原生发现，CLI JSON 契约提供通用边界。
9. 所有协议实现均位于本地 Go `ixf` CLI，不引入 Python，也不依赖 UI 自动化。

## 非目标

- 不建设中央工单代理服务，不由 `10.103.251.42` 代持或代发个人 API Key。
- 不在 CLI 内集成 LLM、自然语言解析或自动补全业务事实；这些属于调用方 Agent 的职责。
- 不允许 Agent 绕过用户确认自动执行 create 或 withdraw apply。
- 首版不单独提供 MCP/HTTP Agent 服务；通用接入边界是本地 CLI 加版本化 JSON，后续 adapter
  可以在不改变 ServiceDesk runtime 的前提下叠加。
- 不实现任意 HTTP/cURL 重放器。
- 不把 ServiceDesk 能力塞进 docs、Messenger 或 Bitable 命令族。
- 不在首版支持审批、转单、派单、认领、评论、挂起、恢复、终止或关闭工单。
- 不把“撤回”实现为 `task_action/revoke`；撤回和终止是不同工作流动作。
- 不要求 Registry 签名或建设发布权限系统。该目录按已确认的可信内部存储处理。
- 不在首版提供 Registry 写入命令；overlay 文件由维护者放入共享目录。

## 考虑过的方案

### 方案一：完全动态发现，不使用 Registry

每次创建都读取 ServiceDesk 当前元数据，按字段显示名或稳定标识直接构造 payload。

优点是没有 profile 过期问题，实现和运维最简单。缺点是无法集中维护字段别名、团队默认
值和必填强化；服务端修改显示名后，Agent 的输入契约也会直接变化。

### 方案二：中央 Registry 保存完整静态 Profile

将 `snapshot_id`、后端字段 key、默认值和完整 payload 模板存入共享目录。

优点是可审计且每次提交结构稳定。缺点是重复保存 ServiceDesk 已经提供的结构信息；任何
模板更新都可能导致 profile 失效，需要重新抓取和发布。

### 方案三：实时元数据加轻量 Overlay

运行时读取最新服务和表单定义；Registry 只保存稳定名称、字段别名、团队默认值和必填强化。

采用方案三。它保留动态适配能力，同时为团队和 Agent 提供稳定、可读的输入契约。方案一
仍作为无 overlay 的通用入口存在；方案二不采用。

## 已验证的 ServiceDesk API

所有请求使用固定 ServiceDesk 主机：
`https://servicedesk.iflytek.com`。

请求头：

```text
businessId: <URL 路径中的业务 ID>
apikey: <用户个人 API Key>
Content-Type: application/json
```

已验证端点：

| 用途 | 方法与路径 |
| --- | --- |
| 验证当前用户和 API Key | `GET /api/user/v3/userinfo/` |
| 获取服务及当前快照 | `GET /api/service/v3/service/{service_id}/` |
| 获取动态表单定义 | `GET /api/tasktmpl3/v3/lading/?service_id=...&snapshot_id=...` |
| 创建工单 | `POST /api/tasktmpl3/v3/start/` |
| 列出当前用户创建的工单 | `GET /api/taskflow3/v3/taskflow/?key=request&flow_type=apply&limit=...&offset=...` |
| 查询工单详情和状态 | `GET /api/taskflow3/v3/taskflow/{ticket_id}/` |
| 撤回工单 | `POST /api/tasktmpl3/v3/withdraw/` |

2026-11-20 已使用个人 API Key-only 验证列表端点，不需要 Cookie：

- `userinfo.data.business` 返回当前用户可访问的 business 列表，每项含 `business_id`。
- 对每个 business 使用对应 `businessId` header，并传 `key=request&flow_type=apply`；返回记录的
  creator 均为当前 userinfo username。
- 分页使用 `limit/offset`，响应 `meta` 含 `limit`、`offset`、`next`、`previous` 和
  `total_count`；记录按 `created_time` 倒序。
- 列表项含 `business_id`、`instance_id`、`name`、`title`、`service_id`、`service_name`、
  `state`、`step`、`current_processors`、`created_time`、`update_time`、`finish_time` 和
  `resource_uri` 等字段。
- business `1` 的结果会聚合部分其他 business，且与专属 business 查询有重复。默认 list 必须
  遍历 userinfo 返回的全部 business，按 `instance_id` 去重，并保留每个 scope 的成功/失败信息。
- 已观察状态包括 `RUNNING`、`FINISHED`、`WITHDRAWN` 和 `REVOKE`；状态分类仍按失败关闭规则
  处理未来未知值。

实现和测试使用脱敏 fixture；不得用 UI 自动化、浏览器抓取结果或逐个猜测 ticket ID 代替该 API。

首版创建只支持已验证的 service key `request`。其他 key 可以 inspect，但 create dry-run
返回 `unsupported_service_key` 并阻止 apply；后续必须用对应类型的真实测试表单验证后才能
扩充 allowlist。

已验证的创建请求外层结构为：

```json
{
  "snapshot_id": "218527",
  "service_id": 4003,
  "key": "request",
  "priority_id": "",
  "impact_id": "",
  "urgency_id": "",
  "source_system": "",
  "source_business": "",
  "inputs": {
    "retry": "true",
    "_loop": 0,
    "<current backend field key>": "<normalized value>"
  },
  "temp_submit_id": null
}
```

撤回请求固定为
`{"instance_id":"<ticket-id>","message":"<reason>"}`。所有端点使用
`{"result":true,"data":...}` 一类 envelope；HTTP 200 但 `result=false` 仍然是失败。

实现的第一项 fixture 工作是把本次已验证的 userinfo、service、lading、list、start、get 和
withdraw 响应整理成脱敏 JSON。service/lading/list fixture 保留真实结构；写入响应只保留实现和
验证所需字段。
401、403、`result=false`、格式错误和超时使用合成 fixture，CI 不读取本机 `/tmp` 文件或
真实 API Key。

这些是当前网页前端使用的内部 API，不视为稳定公开协议。端点或响应结构变化属于
`ixf-toolbox` runtime 兼容性问题；表单字段变化属于元数据/overlay 漂移问题，两者必须在
错误分类中区分。

## Agent Runtime CLI

新增独立顶层命令族 `ixf servicedesk`。所有 Agent 工作流使用 `--json`；不带 `--json` 的 text
模式只用于人工调试。CLI input schema 保持严格并拒绝未知字段；Agent-facing output 在
`contractVersion=1` 内只做向后兼容的字段追加，调用方必须忽略不认识的输出属性。破坏性输出
变更必须提升 `contractVersion`。

### 认证与诊断

```bash
ixf servicedesk doctor --business-id 15 --json
```

API Key 来源优先级：

1. `IXF_SERVICEDESK_API_KEY` 环境变量。
2. `--api-key-file <path>` 指向的单行文本文件。
3. `~/.config/ixf-toolbox/servicedesk-api-key`。

`doctor` 在线 userinfo 探测所需的 business ID 来源优先级为：

1. `--business-id <positive integer>`。
2. `IXF_SERVICEDESK_BUSINESS_ID` 环境变量。
3. `~/.config/ixf-toolbox/servicedesk-business-id` 单行文件。

这些 ambient business ID 来源只用于 standalone doctor 和 root doctor。create、get、withdraw 和
profile validate 必须从 URL、profile 或命令自身的显式参数取得 business ID，不允许被环境变量
静默改写目标。

不提供 `--api-key <value>`，避免密钥进入 shell history 和进程参数。CLI 不负责生成或刷新
API Key；用户继续在 ServiceDesk 个人信息页面获取。默认凭据文件必须是普通文件，Unix
权限宽于 `0600` 时拒绝使用，Windows 只做普通文件和当前用户可读检查。

ServiceDesk 没有公开 API Key 格式规范。CLI 只去除首尾空白并要求非空，不按本次观察到的
40 字符长度或字符集做本地拒绝；有效性以 userinfo 在线探测为准。

`doctor` 只读，报告来源、是否存在以及 userinfo 探测结果，不打印密钥或密钥长度。没有可用
business ID 时仍完成本地凭据检查，将在线探测标记为 skipped，而不是向 userinfo 发送缺少
`businessId` 的请求。
root `ixf doctor --json` 增加 `services.servicedesk` 摘要，但 ServiceDesk 未配置不影响文档等
既有能力的整体可用性。

### 表单发现

```bash
ixf servicedesk template inspect \
  --url 'https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003' \
  --json

ixf servicedesk template inspect \
  --profile server-hardware-repair-test \
  --json

ixf servicedesk profile discover \
  --url 'https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003' \
  --name server-hardware-repair-test \
  --out profile.json

ixf servicedesk profile validate --input profile.json --json
ixf servicedesk profile list --json
ixf servicedesk profile show server-hardware-repair-test --json
```

`template inspect` 的 `--url` 和 `--profile` 必须且只能提供一个。两种模式都读取实时元数据；
profile 模式还加载 overlay 并执行 matcher 和 drift 检查。命令输出字段名称、类型、必填性、
默认值是否存在、完整枚举选项、条件字段摘要和嵌套表格列，不输出 API Key 或原始私有响应。

它必须输出供 Agent 构造 input 的 resolved contract：

```json
{
  "ok": true,
  "operation": "servicedesk_template_inspect",
  "contractVersion": 1,
  "target": {
    "businessId": 15,
    "serviceId": 4003,
    "snapshotId": "218527"
  },
  "profile": {
    "name": "server-hardware-repair-test",
    "source": "network",
    "stale": false
  },
  "inputContract": {
    "fields": [
      {
        "name": "title",
        "aliases": ["ticketTitle"],
        "displayName": "标题",
        "stableId": "title",
        "type": "input",
        "required": true,
        "conditional": false,
        "defaultAvailable": false,
        "supported": true
      }
    ],
    "unsupportedRequiredFieldCount": 0
  },
  "templateDrift": false,
  "contractHash": "sha256:..."
}
```

profile 模式中的 field `name` 使用 overlay 公共名；URL 模式优先使用非空 StableID，否则使用
唯一显示名。每个字段还按需包含 `description`、`format`、`choices`、`columns` 和条件摘要；
choice 必须同时给出 `label` 和可提交的 `value`，table column 使用与 field 相同的公共名称和
类型描述。`supported=false` 或 `unsupportedRequiredFieldCount>0` 时，Agent 可以解释限制，但
create apply 必须失败关闭。`contractHash` 覆盖 resolved contract；create 始终重新获取并校验
实时 contract，不能把 Agent 先前保存的 contract 当作 apply 快照。

`profile discover` 根据实时元数据生成本地 overlay 草稿。自动生成内容包括目标服务、基线
快照、最小字段指纹、稳定字段匹配器和默认 input 名称。它不能凭空判断团队业务语义；
维护者可补充别名、默认值和必填强化后再执行 `profile validate`。`profile validate` 同时执行本地
schema 校验和在线实时元数据匹配，因此需要 API Key；`profile list` 和 `profile show` 只读取
Registry，不需要 ServiceDesk API Key。`profile show` 展示的是静态 overlay，不替代在线
`template inspect --profile`。首版不自动上传 Registry。

`profile list --json` 必须读取 index 指向的 overlay，并为每个 profile 输出 `name`、`description`、
完整 `routing` 摘要和 `target`，供 Agent 做业务模板选择。它不返回默认字段值或完整 baseline。
Registry 不可达时可按缓存规则返回相同摘要并标记 source/stale。

### 创建工单

```bash
# 直接使用实时表单，不加载 overlay
ixf servicedesk create \
  --url 'https://servicedesk.iflytek.com/#/order/order-edit/15?service_id=4003' \
  --input ticket.json \
  --dry-run --json

# 使用中央 overlay
ixf servicedesk create \
  --profile server-hardware-repair-test \
  --input ticket.json \
  --apply --json
```

`--url` 和 `--profile` 必须且只能提供一个。未传 `--apply` 时默认为 dry-run；`--dry-run`
和 `--apply` 互斥。

输入首版使用 JSON，沿用仓库现有结构化输入习惯，不新增 YAML 依赖。字段映射允许直接对象
或 `fields` 包装：

```json
{
  "fields": {
    "title": "IDC 机房服务器硬件故障",
    "faults": [
      {
        "serialNumber": "SN123",
        "machineRoom": "合肥机房",
        "description": "磁盘故障",
        "possibleCause": "SMART 告警"
      }
    ],
    "supplement": "是否改配：否；是否自备备件：否"
  }
}
```

不使用 overlay 时，input key 可使用唯一字段显示名或
`field_unique_Identifier`。显示名重复时必须使用稳定标识；不得按第一个匹配项猜测。嵌套表格列
可使用当前列 key 或表格内唯一列名，重复列名必须使用列 key。

### 列表、查询和撤回

```bash
ixf servicedesk list --mine --view recent --limit 20 --json

ixf servicedesk list --mine --view open --limit 20 --json

ixf servicedesk get \
  --url 'https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015' \
  --json

ixf servicedesk get --ticket-id 3463015 --business-id 15 --json

ixf servicedesk withdraw \
  --ticket-id 3463015 \
  --business-id 15 \
  --reason '问题已自行恢复' \
  --dry-run --json

ixf servicedesk withdraw \
  --ticket-id 3463015 \
  --business-id 15 \
  --reason '问题已自行恢复' \
  --apply --json
```

`list` 是只读操作，首版只允许 `--mine`，不提供查询其他用户。`--view recent` 按创建时间倒序
返回各状态的近期工单；`--view open` 返回已知 active 状态以及无法安全归入 terminal 的未知状态，
避免把仍在处理的新状态静默漏掉。已知 `WITHDRAWN`、`FINISHED` 和 `REVOKE` 属于 terminal；
已验证的 `RUNNING`、`PAUSE` 属于 active。每项输出 ticket ID、工单号、标题、service、原始状态、
`stateCategory`、当前步骤/处理人、创建/更新时间和详情 URL。

默认 list 先读取 userinfo 的 business 列表，再对每个 business 分页请求 `flow_type=apply`，合并后
按 `instance_id` 去重并重新按 `created_time` 倒序。`--limit` 限制的是去重后的最终结果，不是每个
scope 的请求数；实现必须继续取页直到得到足够结果、所有 scope 耗尽或某个 scope 失败。

列表输出必须包含排序方式、查询过的 business scope、分页 cursor、`hasMore` 和 `complete`。如果
某个 scope 请求失败或只取得部分页，`complete=false` 并列出失败 scope；Agent 不得把部分结果
表述为“你的全部工单”。CLI 不提供自然语言 query 或关键词检索；Agent 在结构化列表上结合会话
上下文做语义定位，必要时继续翻页。

`get` 归一化输出工单号、标题、状态、当前步骤、当前处理人、创建人、服务信息和是否处于可撤回
候选状态。状态至少识别 `RUNNING`、`PAUSE`、`WITHDRAWN`、`FINISHED` 和 `REVOKE`，未知
状态原样保留。

`withdraw` dry-run 先读取工单，确认目标 ID、工单号、标题和当前状态。已经
`WITHDRAWN`、`FINISHED` 或 `REVOKE` 的工单拒绝撤回；apply 再次读取状态后调用
`/withdraw/`，最后回读并要求状态为 `WITHDRAWN`。撤回原因去除首尾空白后必须为
1 到 500 个 Unicode 字符。CLI 还必须读取 userinfo，并要求当前 API Key 对应 username 与
工单 creator 一致；首版只允许 `RUNNING` 和 `PAUSE` 状态撤回，不使用管理员角色代撤他人
工单，未知状态失败关闭。不得自动改用终止接口。

## URL 解析

只接受 `https://servicedesk.iflytek.com`：

- 创建 URL：hash path `/order/order-edit/{businessId}`，query 中必须有 `service_id`。
- 详情 URL：hash path `/order/order-detail/{businessId}`，query 中必须有 `id`。

Registry overlay 不允许指定主机或任意 endpoint，只保存 `businessId` 和 `serviceId`。测试通过
注入 `http.Client` 和 fixture server 完成，不为生产 CLI 暴露任意 base URL，从而避免 API Key
被发送到错误主机。

## 实时字段模型

`lading` 返回以当前后端字段 key 为 map key 的对象。CLI 将每个字段归一化为：

```go
type Field struct {
    BackendKey      string
    StableID        string
    Name            string
    Type            string
    Required        bool
    DefaultValue    any
    Choices         []Choice
    Conditions      ConditionSet
    TableColumns    []TableColumn
}
```

其中：

- `BackendKey` 是创建 payload 必须使用、但可能随快照变化的 key。
- `StableID` 来自 `field_unique_Identifier`，作为 overlay 的首选匹配依据。
- 顶层字段匹配顺序为 `StableID`，然后是唯一显示名；不在 overlay 中固化
  `BackendKey`。
- `extend_table` 列优先按列 key 匹配，key 失效时可按唯一列名回退并报告 drift。
- display-only 字段（例如 `hyperlink`）使用实时元数据默认值，不要求用户填写。

### 输入值编码

公共 input JSON 使用以下类型和格式；CLI 先将其规范化，再按实时元数据中的当前后端 key
构造 `inputs`：

| ServiceDesk 字段类型 | 公共 JSON 值 |
| --- | --- |
| `input`、`multi_input`、`textarea` | string |
| `number` | JSON number，不接受数字字符串 |
| `date` | `YYYY-MM-DD` string |
| `time` | `YYYY-MM-DD HH:mm:ss` string |
| `only_time` | `HH:mm:ss` string |
| `date_time_range` | 恰好两个 datetime string 的 array |
| `time_range` | 恰好两个 time string 的 array |
| 单选、单选 checkbox | 一个 string label 或后端 choice value |
| 多选、多选 checkbox | string array，每项为 label 或后端 choice value |
| `switch` | boolean |
| `basis_table`、`extend_table` | object array；每个 object 的 key 按列 matcher 解析 |
| `tui_editor` | plain string，或仅含 `html` 的 object |

选择值同时按后端 value 和 label 做精确匹配；结果去重后必须恰好对应一个选项，否则视为未知或
歧义，不得猜测。解析成功后写入后端 choice value。plain `tui_editor` string 必须先做 HTML
escaping，再包装为 `<p>...</p>`；调用方明确
提供 `{"html":"..."}` 时按富文本 HTML 传递。该 object 不接受其他属性。

字段值为 `null` 表示调用方明确省略该字段，只允许用于非必填字段，并且不再回退到 overlay
或实时默认值；必填字段为 `null` 直接失败。可选 attachment 首版只接受省略、`null` 或空 array，
并规范化为服务端要求的空值；非空附件上传不在首版范围。

默认值优先级固定为：显式 ticket input、overlay `default`、实时 ServiceDesk 默认值、仅在字段
非必填时省略。display-only 字段（例如 `hyperlink`）只使用实时默认值，不接受 input 或 overlay
覆盖。

除 choice label 转换、`tui_editor` 包装、table column key 转换和 `null` 省略外，规范化值以表中
相同 JSON 类型写入 `start.inputs`。表格行 object 的公共列名必须全部解析为当前后端列 key；未知、
重复或缺失的必填列都会阻止 dry-run 成功。

附件实际上传、人员选择、脚本字段、动态数据源字段和无法解释的字段类型不在首版写入范围。
如果它们是必填、收到非空用户输入或影响条件必填计算，dry-run 必须失败；不能透传猜测值。

## 条件字段

元数据中的 `conditions_expression` 可能控制显示和必填性。首版实现已知比较运算符
`==`、`!=`、`in`、`notin`、`>`、`<`、`>=` 和 `<=`，支持条件组的 `and`/`or`。

dry-run 必须基于规范化后的最终输入计算活动字段，并输出：

- `conditionalFieldCount`
- `activeConditionalFieldCount`
- `unsupportedConditionCount`

存在无法解析且可能影响必填或字段可见性的条件时，apply 失败关闭。不能仅依赖 ServiceDesk
返回 400 后再让用户反复试值。

## Overlay 格式

Registry 保存轻量 JSON overlay，不复制完整 lading payload：

下例为结构示意，只展示两个 baseline field；discover 生成的真实文件必须包含实时表单的全部
顶层字段指纹。

```json
{
  "schemaVersion": 1,
  "name": "server-hardware-repair-test",
  "description": "讯飞云服务器硬件报修组内测试",
  "routing": {
    "displayName": "服务器硬件报修",
    "useWhen": "用户需要为服务器等物理设备的磁盘、内存、电源或其他硬件故障申请维修",
    "doNotUseWhen": "问题只涉及软件配置、网络策略、账号权限或应用故障",
    "examples": [
      "提单报修硬件 server-01 sda 硬盘故障",
      "一台机器的盘坏了，帮我发起报修"
    ]
  },
  "target": {
    "businessId": 15,
    "serviceId": 4003
  },
  "baseline": {
    "snapshotId": "218527",
    "serviceKey": "request",
    "metadataHash": "sha256:...",
    "fields": [
      {
        "stableId": "title",
        "name": "标题",
        "type": "input",
        "required": true
      },
      {
        "stableId": "GUZHANGMIAOSHU20220808151539113",
        "name": "故障描述",
        "type": "extend_table",
        "required": false,
        "columns": [
          {"key": "...", "name": "设备SN", "type": "input", "required": true},
          {"key": "...", "name": "所在机房", "type": "input", "required": false},
          {"key": "...", "name": "故障描述", "type": "input", "required": true},
          {"key": "...", "name": "可能原因", "type": "input", "required": false}
        ]
      }
    ]
  },
  "fields": {
    "title": {
      "match": {"stableId": "title", "name": "标题"},
      "aliases": ["ticketTitle"],
      "required": true
    },
    "faults": {
      "match": {
        "stableId": "GUZHANGMIAOSHU20220808151539113",
        "name": "故障描述"
      },
      "columns": {
        "serialNumber": {"match": {"key": "...", "name": "设备SN"}},
        "machineRoom": {"match": {"key": "...", "name": "所在机房"}},
        "description": {"match": {"key": "...", "name": "故障描述"}},
        "possibleCause": {"match": {"key": "...", "name": "可能原因"}}
      }
    },
    "supplement": {
      "match": {"stableId": "QITABUCHONG20220808151806112"}
    }
  }
}
```

`schemaVersion` 必须恰好为 `1`。profile `name` 长度为 1 到 64，只允许匹配
`^[a-z0-9]+(?:-[a-z0-9]+)*$` 的稳定 slug。顶层和所有嵌套 object 都拒绝未知属性，防止拼写
错误被静默忽略。各部分允许的属性固定如下：

- 顶层：`schemaVersion`、`name`、`description`、`routing`、`target`、`baseline`、`fields`。
- `routing`：`displayName`、`useWhen`、`doNotUseWhen`、`examples`。
- `target`：`businessId`、`serviceId`，均为正整数。
- `baseline`：`snapshotId`、`serviceKey`、`metadataHash`、`fields`。
- overlay field：`match`、`aliases`、`description`、`default`、`required`、`columns`。
- table column：`match`、`aliases`、`description`、`default`、`required`。
- baseline field fingerprint：`stableId`、`name`、`type`、`required`、`choiceHash`、`defaultHash`、
  `conditionHash`、`columns`。
- baseline column fingerprint：`key`、`name`、`type`、`required`、`choiceHash`、`defaultHash`、
  `conditionHash`。
- 顶层 matcher：至少包含 `stableId` 或 `name` 之一；两者都有时先匹配 `stableId`，失败后按
  唯一 `name` 回退并报告 drift。
- table column matcher：至少包含 `key` 或 `name` 之一；两者都有时先匹配 `key`，失败后按
  唯一 `name` 回退并报告 drift。

顶层七个属性、`routing` 四个属性、`target` 两个属性以及 `baseline` 四个属性都必填；
`description` 可为空字符串，
`fields` 必须是 object。overlay field 和 table column 的 `match` 必填，其余属性可选。fingerprint
的 `name`、`type`、`required` 必填；顶层 `stableId`、列 `key` 和选择类型 `choiceHash` 在实时
元数据提供时必填；实时默认值存在时 `defaultHash` 必填，实时条件表达式存在时 `conditionHash`
必填；`columns` 只允许且必须用于 table 类型。`snapshotId` 和 `serviceKey` 是非空 string。
matcher 的值、公共 input 名和 alias 均为去除首尾空白后 1 到 128 个 Unicode 字符；aliases 必须是
string array 且不得包含重复项。

`routing.displayName` 是 Agent 向用户展示的工单类型；`useWhen` 用自然语言描述适用的业务目标，
`doNotUseWhen` 描述容易混淆但不适用的边界，`examples` 提供少量典型表达而不是穷举关键词。
前三个字段去除首尾空白后必须非空，最长 500 个 Unicode 字符；`examples` 可以为空，元素去除
首尾空白后必须非空、不可重复且最长 500 个字符。routing 只向 Agent 提供语义上下文，不参与
ServiceDesk payload，也不能覆盖 `target`。CLI 不根据这些内容实现关键词匹配、自动评分或隐式
选单。profile routing 只在 Agent 已确认用户存在工单动作意图后才会读取；它不能把普通故障讨论
升级为提单请求。

`fields` object 的 key 是稳定的公共 input 名；`aliases` 是额外可接受的 input 名。公共名和所有
alias 在顶层作用域必须唯一；每个 table 的 column 公共名和 alias 在该 table 内必须唯一，不同
table 可以复用列名。`default` 必须符合目标字段的公共 JSON 类型。overlay
`required=true` 可以把实时可选字段强化为必填；它不能把服务端必填字段放宽为可选，若对实时
必填字段声明 `required=false`，在线 validate 直接失败。首版不定义通用 validation DSL；除
required、类型、枚举、条件和 ServiceDesk 实时约束外，不接受任意自定义校验属性。
table 字段的 overlay `default` 必须逐行使用该 overlay 声明的公共列名或 alias；未知列、实时
后端列 key、重复解析到同一列、错误 JSON 类型和缺失的实时必填列均使在线 validate 失败。

`baseline.fields` 必须覆盖 discover 时的全部顶层字段，而不只覆盖 overlay 使用的字段。每个字段
保存 `stableId`（不存在时省略）、`name`、`type`、`required`，选择字段额外保存规范化 choice
列表的 `choiceHash`，存在实时默认值或条件表达式时分别保存 `defaultHash`、`conditionHash`；表格
字段的 `columns` 保存每列的 `key`、`name`、`type`、`required`，选择列、默认值和条件表达式同样
保存对应 hash。这组最小指纹用于产生可解释的 drift，不保存原始 lading。`metadataHash` 是完整
规范化指纹的摘要，格式为 `sha256:` 加 64 位小写十六进制；`choiceHash`、`defaultHash` 和
`conditionHash` 使用相同格式，分别对规范化 choice 列表、默认 JSON 值和条件表达式计算。

overlay 不能包含可执行脚本、任意 URL、API Key、Cookie 或自定义 HTTP header。

## Registry 与缓存

默认根地址：

```text
http://10.103.251.43/files/ixf-toolbox/servicedesk/v1/
```

维护端目录：

```text
/disk1/files/ixf-toolbox/servicedesk/v1/
```

目录结构：

```text
v1/
├── index.json
└── profiles/
    └── server-hardware-repair-test.json
```

`index.json` 的完整 schema 为：

```json
{
  "schemaVersion": 1,
  "updatedAt": "2026-11-20T12:00:00+08:00",
  "profiles": [
    {
      "name": "server-hardware-repair-test",
      "path": "profiles/server-hardware-repair-test.json",
      "sha256": "<64 lowercase hex characters>"
    }
  ]
}
```

`schemaVersion` 必须恰好为 `1`，`updatedAt` 必须是 RFC3339 timestamp，profile name 使用与
overlay 相同的 slug 规则并且不可重复。`path` 必须是使用 `/` 的 clean relative path，禁止绝对
路径、`.` 或 `..` 路径段、反斜杠、query 和 fragment；解析后必须仍位于 Registry 根地址下。
`sha256` 是 overlay 原始响应 bytes 的 64 位小写十六进制摘要，只用于传输完整性检查，不作为
安全签名。index、entry 和 overlay 均拒绝未知 JSON 属性；index 中的 name 必须与下载后的
overlay name 相同。

CLI 使用 `os.UserCacheDir()` 下的 `ixf-toolbox/servicedesk/` 缓存最后一次有效 index 和
overlay。在线读取失败时可使用缓存，但 dry-run 和 apply 都要输出：

- `registrySource`: `network` 或 `cache`
- `registryFetchedAt`
- `registryStale`

缓存自成功下载时起 24 小时后标记 stale。任何 hash 和 schema 校验通过的缓存都可用于
`profile list/show` 和 create dry-run；create apply 只允许使用本次网络成功取得的 Registry，
或年龄不超过 24 小时的缓存。超过 24 小时的缓存不因 hash 正确而获准 apply，因为 hash 只能
检测传输损坏，不能证明业务配置仍然有效。

缓存只影响 overlay；create dry-run 和 apply 始终需要从 ServiceDesk 获取实时表单元数据。
没有实时元数据时只允许 `profile list/show`，不得构造看似可提交的 create dry-run 计划，更不能
apply。`profile validate` 同样始终使用实时元数据。

## 漂移检测

加载 overlay 后，CLI 使用实时元数据重新解析所有 matcher，并将当前表单和 baseline 摘要
比较。`snapshot_id` 变化本身只产生 drift，不直接阻止 apply。

非破坏性变化包括：

- 新增可选字段；
- 描述或提示变化；
- 没有伴随类型、条件或其他后果变化的实时默认值 fingerprint 变化；
- matcher 仍唯一解析的显示名变化。

破坏性变化包括：

- overlay 字段或表格列无法解析或出现歧义；
- 字段类型变化；
- 新增必填字段且没有实时默认值或用户输入；
- overlay 默认值不再属于当前枚举；
- 字段或表格列的 `conditionHash` 变化或由存在变为不存在；
- 条件表达式无法解释且影响活动必填字段；
- 当前 ServiceDesk service key 与已支持创建模型不兼容。

dry-run 输出 `templateDrift`、`driftClassification` 和结构化 `driftChanges`。破坏性漂移直接
设置 `apply.blocked=true`，首版不提供跳过该检查的 override flag；应重新 discover 和验证
overlay。

## 创建流程

Dry-run：

1. 读取并验证 API Key 来源。
2. 解析 URL，或从 Registry/cache 解析 profile。
3. 获取当前 service，读取当前 `snapshot_id` 和 service key。
4. 获取 lading 元数据并归一化字段。
5. 合并 overlay 的别名、默认值和 required 强化。
6. 解析 input，拒绝未知、重复和歧义字段。
7. 计算条件字段，校验必填、类型、枚举和嵌套表格列。
8. 构造内存中的最终 `start` payload，但不发送写请求。
9. 输出目标、模板漂移、字段来源和写入计划，不输出 API Key 或完整原始 payload。

Apply：

1. 完整重复 dry-run preflight，不复用过期计划。
2. `POST /api/tasktmpl3/v3/start/`。
3. 从响应取得 ticket ID。
4. `GET /api/taskflow3/v3/taskflow/{ticket_id}/`。
5. 验证回读 ID、标题、service 信息和可识别状态；验证失败时返回失败并保留 ticket ID 和详情
   URL，不能谎报未创建。

创建不是幂等操作。CLI 不在网络超时后自动重试 `start` POST；如果无法判断服务端是否已创建，
返回 `outcome=unknown`，并输出安全的工单标题和本次请求开始时间，要求用户在 ServiceDesk 网页
或通过只读 list 按标题和时间核查后再决定是否重试，避免重复提单。CLI 可以返回 list 中的可能
候选，但不能仅凭标题相似自动断言创建成功，也不能根据超时猜测“未创建”；自动恢复未知结果
留给后续单独设计。

## JSON 输出

创建 dry-run 示例：

```json
{
  "ok": true,
  "operation": "servicedesk_create",
  "contractVersion": 1,
  "dryRun": true,
  "target": {
    "businessId": 15,
    "serviceId": 4003,
    "serviceName": "【讯飞云】服务器硬件报修组内测试"
  },
  "template": {
    "snapshotId": "218527",
    "fieldCount": 7,
    "templateDrift": false
  },
  "input": {
    "providedFieldCount": 3,
    "defaultedFieldCount": 1,
    "tableRowCount": 1,
    "missingRequiredCount": 0,
    "unsupportedFieldCount": 0
  },
  "apply": {
    "blocked": false,
    "requiredFlags": ["--apply"]
  }
}
```

apply 成功增加：

```json
{
  "applied": true,
  "ticket": {
    "id": "3463015",
    "name": "REQ20261120052741",
    "state": "RUNNING",
    "detailURL": "https://servicedesk.iflytek.com/#/order/order-detail/15?id=3463015"
  },
  "verify": {
    "ok": true,
    "titleMatched": true,
    "stateReadable": true
  }
}
```

撤回 apply 成功必须输出撤回前后状态和 `verify.stateWithdrawn=true`。

Agent-facing domain failure 同样输出单个 JSON document，并以非零退出码结束：

```json
{
  "ok": false,
  "operation": "servicedesk_create",
  "contractVersion": 1,
  "error": {
    "code": "missing_required_field",
    "message": "required field is missing: supplement",
    "retryable": false,
    "action": "provide field supplement and run dry-run again",
    "field": "supplement"
  },
  "apply": {
    "blocked": true
  }
}
```

所有预期的认证、权限、Registry、drift、input validation、状态冲突和未知创建结果都必须有稳定
`error.code`、`retryable` 和可执行的 `action`；字段相关错误增加公共 `field`，表格列错误再增加
`column` 和 `rowIndex`。Agent 不应解析 `message` 文本判断下一步。flag/usage 错误继续遵循现有
CLI 约定使用退出码 2；不得因为开启 `--json` 而把 API Key 或原始响应放入错误对象。

## 错误与隐私边界

- API Key、Cookie、认证 header 和原始 userinfo 响应不得进入 stdout、stderr 或测试快照。
- ServiceDesk 401/403 分别归类为认证失败和权限不足，不笼统报告网络错误。
- 元数据协议不兼容与 profile drift 使用不同错误 subtype。
- 创建 POST 超时不得自动重试。
- 服务端返回 ticket ID 后，任何后续验证失败都必须在错误输出中保留 ID 和安全详情 URL。
- dry-run 输出字段是否填写、类型、长度或行数摘要；默认不回显长文本和表格完整内容。
- Registry 文件不可指定 ServiceDesk 之外的目标主机。
- create 返回 `outcome=unknown` 时不得输出原始 payload，只输出人工排查所需的标题、请求时间和
  目标 service 摘要。

## 包与实现边界

新增 `internal/servicedesk`，内部按职责拆分文件而不是创建多个相互循环的小包：

- `client.go`：固定端点、headers、响应 envelope 和 typed API errors。
- `auth.go`：API Key 来源选择和安全诊断。
- `url.go`：创建/详情 URL 解析。
- `metadata.go`：service/lading 获取与规范化。
- `fields.go`：字段类型、条件、表格和 input 解析。
- `overlay.go`：overlay schema、matcher、discover 和 validate。
- `registry.go`：静态 Registry 下载、hash 校验和缓存。
- `create.go`：dry-run、apply 和回读验证。
- `ticket.go`：list、get、withdraw、分页和状态归一化。

`cmd/ixf` 只负责 flag 解析、帮助文本和输出格式，不包含 ServiceDesk payload 细节。HTTP client、
时钟、Registry loader 和凭据 loader 都可注入，保证测试不访问真实服务。

不直接扩展 `internal/cookies` 来读取 Edge ServiceDesk 登录态。此次浏览器 Cookie 仅用于协议
发现；正式 CLI 只接受个人 API Key。

## Agent 调用契约与插件路由

安装后的用户入口是自然语言，而不是命令。`using-ixf-toolbox` 必须将“我要提个工单”等未点名
工具的请求路由到 `ixf-servicedesk-writer`；对应 skill 再按 plugin runtime 解析规则定位
兼容版本的本地 `ixf` 二进制。缺少 API Key 时，Agent 应依据结构化认证错误引导用户配置本机
环境变量或凭据文件，而不是退回网页操作、要求用户抓包，或声称没有工单能力。

进入 ServiceDesk 路由前必须先满足工单意图门槛：用户语义上明确询问支持的工单类型，或要求
提单、报修、创建/发起工单、查询近期/未完成工单、查询单张工单状态或撤回工单。仅出现服务器、
硬盘、故障、损坏、告警等问题内容时不触发；
例如“服务器硬盘坏了，帮我分析原因”继续按普通分析请求处理，而“服务器硬盘坏了，帮我报修”
才进入写操作 skill。该门槛由 Agent 根据完整句意判断，不由 CLI 执行字面关键词匹配。

这里的 reader/writer 是 Agent skill 分类，不是后台服务、独立进程或 API 代理。
`ixf-servicedesk-reader` 约束只读的发现和查询流程；`ixf-servicedesk-writer` 约束 create/withdraw
必须 inspect、dry-run、取得确认后才能 apply。真正读取或修改 ServiceDesk 的始终是本地 Go
`ixf` 二进制。

路由分为两级：

1. 能力路由：`using-ixf-toolbox` 只在存在明确工单能力查询或动作意图时，选择 ServiceDesk 读取
   或写操作 skill；普通故障描述不进入此路由。
2. 业务模板路由：写操作 skill 指导 Agent 通过 `profile list --json` 取得 Registry 中的 routing
   摘要，Agent 根据用户语义选择具体 profile；profile 的 `target.businessId/serviceId` 决定实际
   工单模板。

例如“提单报修硬件 server-01 sda 硬盘”先命中 `ixf-servicedesk-writer` 写操作 skill。Agent 阅读
各 profile 的 `useWhen`、`doNotUseWhen` 和 examples，结合整段对话判断这是物理设备维修需求，
从而选择服务器硬件报修 profile，而不是按“硬盘”字符串命中规则。随后
`template inspect --profile` 返回实时字段契约，Agent 才把 `server-01`、`sda` 和故障描述映射到
title/table/supplement 等公共字段。模板选择和字段映射是两个步骤，skill 中不硬编码
`serviceId` 或当前后端字段 key。

Agent 只有在语义上存在一个明确适用的 profile 时才能自动选择。没有适用项、多个 profile 都
合理或 routing 信息不足时，必须向用户展示候选 `displayName` 和 description 询问；不得选择
列表第一项。CLI 不内置自然语言模型、关键词匹配器或评分器，语义判断完全留在 Agent，Registry
只提供团队维护的工单类型目录和边界说明。

标准 create 调用链为：

1. 路由 skill 根据自然语言意图选择 `ixf-servicedesk-writer` 写操作 skill；Agent 根据用户提供的
   URL 或 `profile list --json` routing 摘要选择明确的 URL/profile，不明确时询问用户，不得凭
   profile 名或列表顺序猜目标。
2. Agent 运行 `template inspect --url/--profile ... --json`，读取当前 `inputContract`。
3. Agent 根据用户已提供的事实构造 input JSON。缺少业务事实时向用户询问，不由 CLI 或 Agent
   编造；事实可以来自当前请求之前的会话上下文，不要求用户重复，有实时默认值的可选字段可以
   省略。
4. Agent 运行 create dry-run，按结构化 error 修正输入，并向用户展示目标 service、工单标题、
   字段摘要、默认值数量、drift 和 apply 是否 blocked。
5. 只有用户明确确认本次目标和内容后，Agent 才运行 create `--apply`；apply 自行重新获取实时
   contract 并完成回读验证。
6. Agent 返回工单号、状态和详情 URL；若 `outcome=unknown`，停止自动重试并按 error action
   引导人工核查。

“支持哪些工单”调用 `profile list`；“最近提了哪些”或“哪些还没完成”调用 ticket `list`，均为
只读操作，可以直接执行。按业务描述撤回时，Agent 先 list 本人近期 active/unknown 工单并语义
定位；找不到时再读 recent 全状态列表，以区分“不存在”和“已经结束”。对唯一明确且可撤回的
ticket ID 才运行 withdraw dry-run。withdraw 必须展示工单号、标题、当前状态和原因，取得本次
明确确认后才 apply；之前对 create 的确认不能复用于 withdraw。

没有安装原生 plugin 的 Agent 也可以按上述 CLI JSON 契约接入，因此业务能力不依赖 Codex 或
Claude Code 专有 API。原生 plugin 的职责是让宿主自动发现路由和安全操作顺序，不复制
ServiceDesk 协议实现。

新增 canonical skills：

- `ixf-servicedesk-reader`：template inspect、profile list/show/validate，以及 ticket list/get。
- `ixf-servicedesk-writer`：create 与 withdraw，强制先 dry-run，再获得明确确认后 apply。

`using-ixf-toolbox` 增加以下触发词和 URL：

- `servicedesk.iflytek.com`
- 明确询问支持的工单类型，或表达提单、报修、创建/发起工单、查单、查询近期/未完成工单、
  查询工单状态、撤回工单意图

Codex/Claude plugin package 继续由现有 `pluginpack` 生成并做 drift 检查。写操作 skill 必须明确
实现上述调用链，区分“撤回”和“终止”，不得因用户说“撤单”调用 revoke/terminate。skill 不得
要求用户把 API Key 粘贴到对话中；认证缺失时只引导用户在环境变量或本机凭据文件中配置。

## Doctor 与 README

README 增加：

- Agentic 工单定位、标准调用链，以及 CLI/plugin/skill 的职责边界；
- “工单意图门槛 -> 业务 profile 路由 -> 实时字段映射”的调用模型；
- 安装后直接用自然语言提单、查单、撤回的用户入口，不要求用户学习 CLI；
- 未安装原生 plugin 的其他本地 Agent 如何通过 `--json` contract 接入；
- ServiceDesk 能力边界和命令速查；
- API Key 获取与三种本地来源；
- Registry/overlay 与实时元数据的关系；
- 支持类型列表、上下文提单、我的近期/未完成工单、按描述撤回示例；
- 不支持的工作流动作和附件限制。

`ixf servicedesk doctor --business-id <id> --json` 是完整在线诊断入口。root doctor 只汇总：

```json
{
  "services": {
    "servicedesk": {
      "configured": true,
      "credentialSource": "env",
      "businessId": 15,
      "apiReachable": true,
      "authenticated": true
    }
  }
}
```

不配置 ServiceDesk 不应让 root doctor 的总体 `ok` 变为 false，因为该能力不是 docs 等命令
的依赖。配置了 API Key 但没有 doctor business ID 时，`apiReachable` 和 `authenticated` 为
`null`，并输出 `probeSkipped=business_id_required`；这同样不改变 root doctor 的总体 `ok`。

## 测试策略

### 单元测试

- URL 解析：创建 URL、详情 URL、错误主机、缺少 ID、非法数字。
- API Key 与 doctor business ID：来源优先级、空值、文件权限、跳过在线探测、输出脱敏。
- metadata：service/lading envelope、字段规范化、嵌套表格、默认值和未知类型。
- Agent contract：URL/profile 两种 resolved contract、`contractVersion`、choice/table 描述、
  unsupported required field 和 contract hash。
- input：所有支持类型、`null`、富文本、显示名/StableID/overlay 别名解析、重复名、缺失必填、
  枚举错误和 table column key 转换。
- conditions：已知运算符、and/or、未知表达式失败关闭。
- overlay：discover、validate、matcher 和 drift 分类。
- profile routing：list 摘要以及 `useWhen/doNotUseWhen/examples` schema；CLI 不提供关键词
  matcher 或 profile resolver。
- Registry：network/cache、hash 不匹配、24 小时 apply 阈值、路径穿越和 schema version。
- create：dry-run 零写入、apply 请求形状、回读成功、回读失败保留 ticket ID、POST 超时不重试
  并返回 `outcome=unknown`。
- list：mine-only、recent/open 视图、状态分类、创建时间排序、分页、跨 scope 部分失败和
  `complete=false`。
- withdraw：状态 preflight、禁止已结束状态、回读 `WITHDRAWN`。
- Agent error：稳定 code/retryable/action、字段定位和非零退出码，不依赖 message 文本。
- privacy：所有错误和 JSON 输出均不包含 API Key。

### CLI 与集成测试

- 帮助命令覆盖所有子命令和互斥 flag。
- 使用 `httptest.Server` 模拟已验证的 ServiceDesk 响应，不访问真实内网。
- 使用脱敏后的 `service_id=4003` service/lading fixture 覆盖真实字段形状。
- Registry fixture server 覆盖 index、overlay 和缓存回退。
- plugin package `--check` 验证 canonical skill 没有漂移；skill fixture 使用未提及 Toolbox 的
  “我要提个工单”“查一下这个工单”“撤回这个工单”验证自动路由到正确 skill，并使用不复述
  profile 原文的表达验证 Agent 语义选单和歧义询问，同时保持 inspect/dry-run/确认/apply 顺序；
  “服务器硬盘坏了，帮我分析原因”等无工单动作意图的负例不得触发 ServiceDesk skill。
- Agent 场景 fixture 覆盖“支持哪些工单”“报修上面的问题”“撤回 xx 硬盘的报修单”“最近提了
  哪些”“哪些还没完成”，并验证上下文复用、语义定位和多候选询问。
- 完整运行 `go test ./...`，并保持 macOS、Linux、Windows CI 通过。

真实验收使用专用测试表单：先列出支持类型，使用前文上下文完成 dry-run，由用户确认后创建一张
明确标注的测试工单；在 recent/open 列表中找到它，按业务描述定位并查询为 `RUNNING`，再撤回
并确认 `WITHDRAWN`，最后确认它不再出现在 open 结果中。不得在 CI 中使用真实 API Key。

## 实施顺序

1. 整理已验证的 userinfo/service/lading/list/start/get/withdraw 脱敏 fixture。
2. ServiceDesk client、API Key loader、URL parser 和 fixture。
3. service/lading 元数据模型与 URL/profile resolved `template inspect` contract。
4. overlay schema、discover/validate 和 Registry/cache。
5. create input 解析、dry-run 和漂移检测。
6. create apply 与回读验证。
7. ticket list/get 与 withdraw dry-run/apply。
8. root doctor、README、skills 和 plugin package 更新。
9. 在 `/disk1/files/ixf-toolbox/servicedesk/v1/` 发布 index 和第一个已验证 overlay。
10. 真实测试表单端到端验收并发布功能版本。

第一个 overlay 由本次实现生成、在线 validate 并随功能版本发布，目标为 `businessId=15`、
`serviceId=4003` 的测试表单。它只包含稳定公共名、匹配器、描述和 discover 基线，不加入未经
用户确认的业务默认值或额外 required 强化；routing 元数据包含已确认的服务器硬件报修适用
范围、排除边界和语义示例，不包含关键词匹配规则。用户是首版 Registry 的唯一维护者；CLI 不提供
上传、并发写入或发布审批能力。

该能力属于功能新增，按项目版本规则提升倒数第二位版本号，而不是仅提升 patch 位。

## 待实现者决定

以下细节可在不改变产品行为的前提下由实现者决定：

- 内部 Go struct 和文件的具体命名；
- metadata hash 的规范化 JSON 编码细节；
- Registry HTTP 缓存的精确超时时间和 User-Agent；
- text 模式输出排版；
- fixture 的最小脱敏字段集合。
