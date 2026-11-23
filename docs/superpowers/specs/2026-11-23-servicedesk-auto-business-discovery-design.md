# ServiceDesk 自动业务域发现设计

## 意图

用户只配置自己的 ServiceDesk API Key 后，就应能查询待办、已提交工单和上下文候选，不能再被要求额外提供一个与查询目标无关的“入口 business ID”。当前 CLI 在调用身份接口前强制加载该配置，和已验证的线上协议不一致。

## 已验证协议

- `GET /api/user/v3/userinfo/` 只携带 API Key 即可返回当前用户可访问的业务域；携带 `businessId` 时结果相同。
- `GET /api/taskflow3/v3/taskflow/` 不携带 `businessId` 会返回空结果，因此必须按 userinfo 返回的每个业务域逐个查询。
- `get`、`create` 和 `withdraw` 的业务域仍来自工单 URL、profile 或目标工单，不使用自动发现值替代明确目标。

## 设计

`Client.UserInfo(ctx, businessID)` 接受 `businessID == 0`，此时省略 `businessId` header；负数仍视为非法。普通请求底层继续要求正数，避免列表和写接口意外变成无业务域请求。

`QueryTickets` 接受 `BootstrapBusinessID == 0` 并调用无 header 的 userinfo。CLI 的 `--business-id`、`IXF_SERVICEDESK_BUSINESS_ID` 和默认文件继续作为兼容和诊断覆盖，但都不再是 `list`、`search` 或 doctor 的前置条件。JSON contract v1 保持不变，自动发现时 `bootstrapBusinessId` 为 `0`。

doctor 在 API Key 可用但没有 business ID 时也执行 userinfo 探测；成功时报告 API 可达和认证成功，不再报告 `business_id_unconfigured`。

## 取舍

考虑过继续要求用户配置一个入口 ID，以及在 CLI 中猜测常见 ID。前者暴露了服务端并不要求的内部参数并阻断装完即用，后者不完整且会制造错误范围。采用无 header 的 userinfo 自动发现，因为它直接使用服务端提供的权限边界，同时保留显式 ID 作为兼容覆盖。

## 非目标

- 不修改列表、详情或写接口对正 business ID 的要求。
- 不改变业务域聚合、分页、去重、排序或 Agent 候选选择语义。
- 不删除已有 business ID flag、环境变量和配置文件支持。

## 验收

- 仅配置 API Key 时，`servicedesk doctor`、`list` 和 `search` 可工作。
- userinfo 自动发现请求不携带 `businessId`，后续每个列表请求携带发现出的实际 ID。
- 显式 business ID 的现有行为继续工作。
- root doctor 不再因缺少 business ID 跳过 ServiceDesk 在线探测。
