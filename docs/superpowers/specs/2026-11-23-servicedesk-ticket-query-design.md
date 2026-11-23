# ServiceDesk 工单查询语义设计

日期：2026-11-23

状态：已审批

## 背景

首版 ServiceDesk CLI 只有 `ixf servicedesk list --mine --view recent|open`。
它固定请求 `flow_type=apply`，因此实际查询的是当前用户提交的工单；`open` 只是在客户端排除
已结束状态。这个命令无法表达“当前流程在我这里、需要我处理的工单”，也没有为 Agent 根据
对话上下文定位某一张工单提供独立的候选检索契约。

线上验证还确认，`userinfo` 返回当前用户可访问的多个 business。查询命令需要以一个可访问的
business ID 作为身份探测入口，然后遍历这些 business；入口 business ID 不代表结果只属于该
business。各 business 结果可能重复，必须按工单实际 business ID 和 instance ID 去重。

本文是对已审批的 `2026-11-20-servicedesk-api-profile-design.md` 中“列表、查询和撤回”读取部分
的增量替代。原设计的创建、撤回、Profile 和安全约束保持不变。

## 意图

让用户通过自然语言完成三类彼此不同的读取操作：

1. 根据当前对话定位某一张工单，并读取详细流转进度。
2. 查询当前流程节点需要本人处理的待办工单。
3. 查询本人提交且尚未完成的工单。

Agent 负责理解上下文、生成检索词和在候选中做语义选择；Go CLI 只负责确定性的 API 查询、
分页、去重、状态归一化和结构化输出。CLI 不内置 LLM、关键词路由表或语义评分。

## 用户体验

### 根据上下文查询某张工单

用户可以说“看看刚才那块硬盘的报修进度”或“查一下 server-01 那张单”。Agent 从当前会话提取
标题线索、设备 SN、主机名、磁盘名、故障描述或已知工单号，调用候选搜索。

候选选择优先级为：

1. 工单号、详情 URL、设备 SN、主机名等明确标识完全匹配。
2. 标题和当前对话的业务语义相关性。
3. 用户询问“当前进度”时，未结束状态优先于已结束状态。
4. 相关性相同时，更新时间较近者优先；没有更新时间时按创建时间。

Agent 自动选择最可能且最近的一张，不要求用户先确认。选择后必须再调用单张工单详情接口，
不能把列表摘要当作最终进度。回答中应说明实际选中的工单号和标题，便于用户发现误匹配。
没有可信候选时明确返回未找到，不选择低相关候选凑结果。

上下文没有说明该工单是本人提交还是当前待办时，Agent 同时搜索 submitted 和 todo，再合并
候选；不能默认只搜其中一类，也不因此询问用户。

自动选择只适用于读取。撤回等写操作可以复用候选检索，但必须展示精确工单并取得本次明确
确认；不能因一次读取时自动选中过目标，就跳过后续写操作确认。

### 查询我的待办工单

用户问“我有哪些待办工单”“现在有哪些单子流程在我这里”时，Agent 查询 `flow_type=wait`。
该结果表示当前用户是当前处理人，不等同于用户创建的工单，也不再按 creator 做二次过滤。

待办结果默认按更新时间倒序，输出工单号、标题、服务、状态、当前步骤、当前处理人、创建人、
创建/更新时间和详情 URL。`flow_type=wait` 的结果本身就是当前待办，不再叠加 submitted 的
open 状态过滤。

### 查询我提交但未完成的工单

用户问“我提交的单还有哪些没完成”“我报修的工单哪些还在处理”时，Agent 查询
`flow_type=apply`，并使用 open 状态过滤。

已知 `WITHDRAWN`、`FINISHED` 和 `REVOKE` 属于 terminal；已知 `RUNNING`、`PAUSE` 属于
active。未来未知状态必须保留在 open 结果中并标记 `stateCategory=unknown`，避免静默漏掉仍在
处理的新状态。

## 方案选择

### 方案一：CLI 内置关键词和模糊评分

CLI 根据标题分词、关键词权重和时间计算候选分数，然后直接返回唯一工单。

优点是所有调用方结果一致。缺点是无法使用完整对话语义，需要持续维护中文分词和业务关键词，
也与“CLI 是确定性执行底座、Agent 负责自然语言理解”的现有架构冲突。

### 方案二：CLI 返回结构化候选，Agent 做语义选择

CLI 接收 Agent 提取的查询文本，跨 business 调用 ServiceDesk 搜索并返回完整候选；Agent 结合
会话上下文自动选取最可能且最近的一张，再调用详情查询。

采用方案二。它保留 API 查询和分页的确定性，同时让语义理解留在最适合的位置。CLI 输出足够
的标题、状态和时间信息，Agent 不需要读取原始响应或猜测 ticket ID。

### 方案三：引入向量索引或中央检索服务

把工单标题和摘要同步到中央索引，通过 embedding 检索候选。

该方案可支持更复杂的历史语义搜索，但需要额外服务、数据同步和权限边界。当前 ServiceDesk
已有查询接口，团队规模和场景不值得引入这层基础设施，因此不采用。

## CLI 契约

### 角色化列表

```bash
ixf servicedesk list \
  --mine \
  --role todo \
  --business-id 9 \
  --limit 20 \
  --json

ixf servicedesk list \
  --mine \
  --role submitted \
  --state open \
  --business-id 9 \
  --limit 20 \
  --json
```

`--role` 必填：

- `todo` 映射到 `flow_type=wait`。
- `submitted` 映射到 `flow_type=apply`。

`--state` 只适用于 `submitted`：

- `all` 返回各状态近期记录。
- `open` 返回 active 和 unknown，排除已知 terminal。

`submitted` 默认 `--state all`。`todo` 不接受 `--state`，避免制造服务端并不存在的组合语义。
首个版本尚未发布，现有 `--view recent|open` 不作为长期兼容接口；实现时直接替换为 role/state，
并同步 README、skill 和命令帮助。

### 候选搜索

```bash
ixf servicedesk search \
  --mine \
  --role any \
  --query 'server-01 硬盘' \
  --state all \
  --business-id 9 \
  --limit 50 \
  --json
```

`search` 与 `list` 使用相同的 role/state、business 遍历、分页和去重逻辑。search 的 `--role`
支持 `submitted`、`todo` 和 `any`，默认 `any`；`any` 分别执行 apply 和 wait 查询并按实际
business/id 去重，同时在候选中保留 `roleMatches`。`--query` 是非空
Unicode 文本，由 Agent 从对话提取；CLI 不扩展同义词，不计算语义分数，也不自动选择候选。
CLI 优先使用 ServiceDesk 的 `q` 参数做服务端过滤。实现前必须用 API Key-only 请求验证 `q` 对
标题、工单号和服务名的实际匹配范围；如果服务端语义不足，只允许在已取得的本人结果上做明确、
可解释的标题/工单号文本过滤，不引入浏览器 Cookie 或 UI 自动化。

本地文本过滤规则固定为：对 query、`name`、`title` 和 `serviceName` 使用 Go Unicode 小写化，
按 Unicode 空白切分 query；每个非空 token 都必须作为子串出现在三个候选字段拼接后的文本中。
不做中文分词、拼音、同义词、编辑距离或 Unicode 规范化。协议验证若证明服务端 `q` 不能覆盖
这三个字段，search 必须完整读取对应 role 的未过滤列表后应用上述本地规则；不能只接受一个
已知低召回的服务端结果集。Agent 可以发起多个不同 query，但每次 CLI 结果都遵循同一规则。

2026-11-23 使用 API Key-only 请求完成协议探测：`q` 能按完整展示工单号和完整标题命中目标，
但不能按完整服务名命中。因此首版 search 固定完整读取对应 role 的未过滤列表，再应用本地文本
规则；不使用 `q` 作为候选集来源。未来只有在服务端检索可证明覆盖 `name`、`title` 和
`serviceName` 后，才允许在不降低召回率的前提下优化为服务端过滤。

search 使用 `role=any,state=open` 时，open 只过滤 submitted 分支；todo 分支本身就是当前可操作
待办，全部保留。`role=any,state=all` 则保留全部 submitted 和 todo。`state` 与 `role=todo` 单独
使用仍是非法组合。

首版 search 不批量读取每个候选的详情，也不承诺搜索只存在于表单 inputs 中的 SN、磁盘名或
故障描述。Agent 可以从上下文生成多个可能的标题查询词；如果这些线索没有出现在标题、展示
工单号或服务名中，允许返回未找到。这样避免为语义召回遍历并暴露大量详情内容。

搜索结果按服务端相关性不可得时的稳定规则排序：`update_time` 倒序，其次 `created_time` 倒序，
最后按 business ID 和 instance ID 保证确定性。语义相关性排序由 Agent 完成。

### 身份探测 business ID

`list` 和 `search` 需要一个当前用户可访问的 business ID 调用 `userinfo`。来源优先级为：

1. `--business-id`。
2. `IXF_SERVICEDESK_BUSINESS_ID`。
3. `~/.config/ixf-toolbox/servicedesk-business-id`。

这个值只用于身份探测，不能限制最终查询 scope，也不能改写候选工单自身的 business ID。
本设计将原先仅供 doctor 使用的 ambient business ID 扩展到只读 list/search；create、get、
withdraw 和 profile validate 仍必须使用显式目标。输出必须明确区分 `bootstrapBusinessId` 和实际查询过的 `scopes`，防止 Agent 把入口值解释为
结果范围。未配置入口 business ID 时失败并给出可执行提示，不遍历猜测常见 ID。

### 单张工单详情

候选确定后继续使用现有接口：

```bash
ixf servicedesk get --business-id 9 --ticket-id 123456 --json
```

Agent 必须使用候选自身的 `businessId` 和内部数字 `id`，不能沿用身份探测入口 business ID。
用户可见的 `REQ...` 是展示工单号 `name`，不是详情 API 所需的内部 ID；提供 `REQ...` 时必须先
用 search 精确匹配。只有详情 URL，或同时给出 business ID 和内部数字 ID 时，才跳过 search。
`get` 输出当前状态、状态分类、步骤、当前处理人和更新时间；是否允许撤回继续由 withdraw
dry-run 根据所有权和实时状态判断，不在读取契约中增加不完整的 `canWithdraw` 推断。

## 聚合与分页

首版为保证跨 business 排序和 open 状态过滤正确，必须完整拉取每个 scope 的所有分页，再做
去重、过滤和全局排序。不同 scope 可以使用小规模有界并发降低等待时间，同一 scope 内仍按
服务端 `next/offset` 顺序分页。

`--limit` 是去重和过滤后的全局输出上限。`hasMore=true` 表示完整结果集中仍有符合条件但因
limit 未返回的工单；`complete=true` 表示所有 scope 的所有分页均成功读取。因此
`complete=true` 可以和 `hasMore=true` 同时出现。

首版不提供 `--cursor`，也不在 JSON 中输出无实际作用的 cursor。未来只有在定义并实现可恢复的
跨 scope cursor 后才能增加该能力。任一 scope 中途失败时保留已取得的部分结果，标记
`complete=false`；这些结果不能被描述为全部。`complete=false` 时无法证明是否还有结果，
`hasMore` 固定输出 JSON `null`；只有 `complete=true` 时 `hasMore` 才是准确的 boolean。

同一 `(business_id, instance_id)` 可能从聚合 business、专属 business、apply 和 wait 多次返回。
去重时合并所有来源的 `roleMatches`，记录内容按以下顺序确定：更新时间较新的优先；时间相同时，
请求 header business 与响应 business 相同的专属 scope 优先；仍相同时 wait 优先于 apply；最后
按请求 scope business ID 从小到大保证确定性。无法解析的更新时间视为最旧。

## JSON 输出

`list` 和 `search` 继续使用顶层 envelope 与 `contractVersion=1`，在 result 中至少包含：

```json
{
  "role": "submitted",
  "stateFilter": "open",
  "query": "server-01 硬盘",
  "bootstrapBusinessId": 9,
  "tickets": [],
  "scopes": [],
  "failedScopes": [],
  "sort": "update_time_desc",
  "hasMore": false,
  "complete": true
}
```

每个 ticket 至少包含 `businessId`、内部数字 `id`、展示工单号 `name`、`title`、`serviceId`、
`serviceName`、`roleMatches`、`state`、
`stateCategory`、`step`、`creator`、`currentProcessors`、`createdTime`、`updateTime` 和 `detailUrl`。

`scopes` 每项使用 `{businessId, flowType, ticketCount, complete, error?}`；`role=any` 时同一
business 分别包含 apply 和 wait 两项。`failedScopes` 使用
`{businessId, flowType, error}`，不能只输出 business ID。任一 scope 失败或分页中断时
`complete=false`；Agent 不得把部分结果表述为全部待办或全部未完成工单。去重键为工单响应中的
实际 `business_id + instance_id`，不能只使用请求 header 的 business ID。

## Agent 行为

ServiceDesk reader skill 根据用户意图区分：

- “待办”“需要我处理”“流程到我”：`list --role todo`。
- “我提交的”“我报修的”“还没完成”：`list --role submitted --state open`。
- “这张单”“上面那个报修”“某设备的工单进度”：先 `search --role any`，Agent 自动选取候选，
  再 `get`。
- 已提供详情 URL，或内部数字 ID 和 business ID：跳过 search，直接 `get`。
- 已提供 `REQ...` 展示工单号：先 search 精确匹配，再用结果中的内部 ID 调用 get。

这些示例用于向 Agent 描述意图，不在 Go CLI 中实现关键词匹配。用户只描述故障但没有查询、
提单、报修、撤回等工单动作意图时，不应触发 ServiceDesk。

自动选择后，Agent 回答应包含选中的工单号和标题；若结果 `complete=false`，必须同时说明查询
不完整。没有可信候选时停止并说明未找到，不能逐个猜测 ticket ID。

## 安全和隐私

- 所有请求只使用个人 API Key；不读取、保存或重放浏览器 Cookie、SSO token 或 HAR。
- API Key 不进入命令参数、JSON 输出、日志、fixture 或 Agent prompt。
- list/search/get 均为只读，可以由 Agent 自动执行。
- create/withdraw 等写操作继续要求 dry-run、精确目标、本次用户确认和显式 `--apply`。
- 搜索仅覆盖当前 API Key 可见的本人提交或本人待办结果，不提供任意用户搜索。
- 测试 fixture 必须脱敏，不保存真实标题、人员、设备标识或原始响应。

## 错误和完整性

沿用稳定 DomainError 分类。新增或明确以下情况：

- `invalid_request`：role/state 组合非法、query 为空或 business ID 未配置。
- `authentication_failed` / `permission_denied`：API Key 无效或 scope 不可访问。
- `invalid_response`：列表记录或分页结构不兼容。
- scope 局部失败不使整个命令 envelope `ok=false`，但 result 必须 `complete=false`。
- 没有任何 scope 完成且没有取得任何 ticket 时返回 `query_failed`，envelope `ok=false`，不能
  返回看似成功的空列表。
- 只要至少一个 scope 完成，或失败前取得了部分 ticket，就返回 `ok=true,complete=false` 和部分
  结果；此时 `hasMore=null`，Agent 必须披露不完整。
- userinfo 返回零个 business 时返回成功、完整的空结果，表示当前身份没有可查询 scope。
- `get` 失败仍是单目标失败，envelope `ok=false`。

Agent 只有在 `complete=true` 时才能声称“全部”；部分结果仍可用于自动选择单张候选，但回答必须
披露查询不完整，且不能基于部分结果执行撤回。

## 验收标准

1. `role=todo` 使用 `flow_type=wait`，能返回当前用户待办且不按 creator 误过滤。
2. `role=submitted,state=open` 使用 `flow_type=apply`，排除已知 terminal 并保留 unknown。
3. search 跨 userinfo scopes 查询、分页、按实际 business/id 去重，并返回标题候选。
4. Agent 能从会话上下文生成 query，自动选择最可能且最近的候选，然后调用 get。
5. 已提供精确 URL 或 ID 时不执行不必要的 search。
6. 任一 scope 失败时输出 `complete=false`，Agent 不宣称结果完整。
7. 读取时自动选择不会削弱 withdraw 的精确目标和确认要求。
8. 全流程仅使用 API Key，不依赖 Cookie、浏览器或 UI 自动化。
