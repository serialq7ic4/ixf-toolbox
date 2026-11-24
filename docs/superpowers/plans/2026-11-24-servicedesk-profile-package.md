# ServiceDesk Profile Public Go Package Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 ServiceDesk profile 契约抽取为可供 `ixf-toolbox` 和独立 Maintainer 共用的纯 Go package，并增加确定性编码、Registry 读取和 rebase 能力。

**Architecture:** `internal/servicedeskapi` 只负责 ServiceDesk 元数据传输，`pkg/servicedeskprofile` 负责无副作用的 profile 领域契约，现有 `internal/servicedesk` 保留凭据、本地缓存和真实工单操作。抽取先保持行为不变，再分阶段增加生产者所需能力。

**Tech Stack:** Go 1.24、标准库 `net/http`/`encoding/json`/`crypto/sha256`、现有 Go 测试套件。

**Spec:** `/Users/chenqian/Project.localized/ixf-servicedesk-profile-maintainer/docs/superpowers/specs/2026-11-24-servicedesk-profile-maintainer-design.md`

## Global Constraints

- Go-only；不引入 Python、UI 自动化或新的第三方依赖。
- `cmd/ixf` 的 JSON、错误码、退出码和 contract hash 必须保持兼容。
- 公共 package 不读取环境变量、凭据文件、缓存目录或本地 Agent 配置。
- 公共元数据客户端只公开 `Service` 和 `Lading`，不能公开真实工单写入或实例读取方法。
- 测试 fixture 必须脱敏，并保持 Windows 换行兼容。

## Review Focus

- 未知 JSON 字段、`null`、trailing JSON 和非法 fingerprint 必须继续严格失败，由 Task 1 的 draft/decode 兼容测试固定。
- HTTP 重定向、非 2xx、超大响应和代理环境不得绕过 Registry/metadata 边界，由 Task 2 的 HTTP 测试固定。
- rebase 的字段/列重名、删除和类型变化不能静默丢失定制，由 Task 3 的 unresolved 测试固定。
- 相同输入的 profile、index 和 rebase 输出必须逐字节/深度确定，由 Task 2 和 Task 3 的重复执行测试固定。
- 抽取后现有 CLI 所有 ServiceDesk 行为必须不变，由每个任务的 `go test ./...` 和最终 smoke test 固定。

---

## 目标

把 ServiceDesk profile 契约从 `internal/servicedesk` 抽取为可被外部模块导入的
`github.com/serialq7ic4/ixf-toolbox/pkg/servicedeskprofile`，让 `ixf-toolbox`（消费者）和
`ixf-servicedesk-profile-maintainer`（生产者）共用同一份实现：

- profile 严格解码、schema 校验、确定性编码和 SHA256；
- ServiceDesk 编辑页 URL 解析；
- 实时 service/lading 元数据读取与归一化；
- discover、validate、matcher、drift 分类和公共输入契约解析；
- Registry index 解码、编码、路径规则和 HTTP 读取校验；
- 确定性的 `RebaseOverlay`。

`ixf-toolbox` 自身改为使用该 package。CLI 行为、JSON 契约（`contractVersion: 1`）和全部现有测试保持不变。

## 硬性约束

1. 无行为变化：`cmd/ixf` 的输出、错误码、退出码、contract hash 与抽取前一致。现有测试不得删除，只能随代码迁移；
   迁移前后 `func Test` 总数不减少。
2. `pkg/servicedeskprofile` 只允许导入标准库和 `internal/servicedeskapi`。不得导入 `cmd/...`、
   `internal/servicedesk` 或任何读取本地配置、凭据文件、环境变量、缓存目录的代码。
3. 公开 API 不暴露工单创建、处理、查询实例或撤回能力。`MetadataClient` 的方法集只有 `Service` 和 `Lading`。
4. `go.mod` 不新增依赖，`go 1.24.0` 不变。
5. 测试只用合成数据（CONTRIBUTING 隐私规则）。不得把线上 Registry 的真实 profile 或 index 提交为 fixture；
   可以在本地临时文件上做一次兼容性核对，但不入库。
6. Windows CI 必须通过：新增 testdata 在 `.gitattributes` 中声明 `text eol=lf`，或测试对换行做归一化。
7. 每个阶段结束都要通过：`go test ./...`、`go vet ./...`、`go run ./cmd/pluginpack --check`、
   `CGO_ENABLED=0 go build -trimpath -o /tmp/ixf-go ./cmd/ixf` 和
   `scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"`。

## 目标布局

```text
internal/servicedeskapi/      共享传输层：apikey 请求、响应信封、APIError、超时/状态分类、
                              密钥脱敏、响应大小限制、ServiceDesk hash 路由 URL 解析、
                              service/lading 两个元数据端点
pkg/servicedeskprofile/       公共 profile 契约（下方 API）
internal/servicedesk/         凭据、工单客户端（userinfo/start/list/get/withdraw）、create、
                              fields、conditions、query、ticket、errors、Registry 本地缓存加载
```

依赖方向：`servicedeskapi` ← `servicedeskprofile` ← `internal/servicedesk` ← `cmd/ixf`。
`internal/servicedesk` 也可以直接导入 `servicedeskapi`。`servicedeskapi` 不导入另外两个包。

`internal/servicedesk.Client` 继续提供 `Service`、`Lading`（委托给 `servicedeskapi` 的同一端点实现），
因此满足 `servicedeskprofile.MetadataSource`。元数据端点的请求构造只存在于 `servicedeskapi` 一处。
`DetailTarget`、`ParseDetailURL` 留在 `internal/servicedesk`，复用 `servicedeskapi` 的 hash 路由解析器。

迁移后的内部代码直接使用限定名（例如 `sdp.Template`），不保留类型别名兼容层。
`func (ResolvedOverlay) FieldCount()` 这类定义在被迁移类型上的方法要删除或迁入公共包；
`create.go` 中仅用于调用它的空 `if` 块一并删除。

## 公共 API

```go
package servicedeskprofile

// URL
type EditTarget struct{ BusinessID, ServiceID int }
func ParseEditURL(raw string) (EditTarget, error)
func (t EditTarget) URL() string // https://servicedesk.iflytek.com/#/order/order-edit/<businessId>?service_id=<serviceId>

// 元数据读取
type APIError = servicedeskapi.APIError
type Service = servicedeskapi.Service
type Lading = servicedeskapi.Lading
type MetadataSource interface {
	Service(ctx context.Context, target EditTarget) (Service, error)
	Lading(ctx context.Context, target EditTarget, snapshotID string) (Lading, error)
}
type ClientOptions struct {
	HTTPClient *http.Client      // 可选；会被克隆，Jar 置空，禁止重定向
	Transport  http.RoundTripper // 可选；覆盖 HTTPClient 的 Transport
}
type MetadataClient struct{ /* unexported */ }
func NewMetadataClient(apiKey string, options ClientOptions) *MetadataClient
func (c *MetadataClient) Service(ctx context.Context, target EditTarget) (Service, error)
func (c *MetadataClient) Lading(ctx context.Context, target EditTarget, snapshotID string) (Lading, error)

// 实时表单
type Choice, TableColumn, Condition, ConditionSet, Field, Template // 字段与 JSON tag 不变
func LoadTemplate(ctx context.Context, source MetadataSource, target EditTarget) (Template, error)
func BuildTemplate(target EditTarget, service Service, lading Lading) (Template, error) // LoadTemplate 去掉网络部分
func IsTableFieldType(fieldType string) bool
func PreferredFieldName(field Field) string // stableId 非空用 stableId，否则用 name

// Overlay / profile
const OverlaySchemaVersion = 1
func IsValidProfileName(name string) bool
type Overlay, Routing, OverlayTarget, Baseline, BaselineField, BaselineColumn,
	FieldMatcher, ColumnMatcher, OverlayField, OverlayColumn
func DecodeOverlay(data []byte) (Overlay, error)      // 行为不变：严格解码 + ValidateOverlaySchema
func DecodeOverlayDraft(data []byte) (Overlay, error) // 新增：与 DecodeOverlay 相同的严格结构解码，但不调用 ValidateOverlaySchema
func ValidateOverlaySchema(overlay Overlay) error
func DiscoverOverlay(template Template, name string) Overlay
func EncodeProfile(overlay Overlay) ([]byte, error)   // 新增，见下

// Drift 与解析
const (
	DriftNonDestructive = "non_destructive"
	DriftDestructive    = "destructive"
)
type ResolvedOverlay, ResolvedField, ResolvedColumn, DriftReport, DriftChange
func CompareBaseline(baseline Baseline, template Template) DriftReport
func ResolveOverlay(overlay Overlay, template Template) (ResolvedOverlay, error)
func ValidateOverlay(template Template, overlay Overlay) (ResolvedOverlay, error)

// 公共输入契约
const ContractVersion = 1
type InspectRequest struct {
	Target  EditTarget
	Profile *ProfileContext
}
type ProfileContext, ResolvedTarget, InputColumn, InputField, InputContract, ResolvedProfile, ResolvedContract
func Inspect(ctx context.Context, source MetadataSource, request InspectRequest) (ResolvedContract, error)
func InspectTemplate(template Template, profile *ProfileContext) (ResolvedContract, error) // 新增：Inspect 去掉网络部分
func ResolveProfileInputContract(fields []Field, overlay ResolvedOverlay) (InputContract, error)
func ProjectProfileDrift(report DriftReport, fields []Field, profile InputContract) (DriftReport, error)

// Registry
const RegistrySchemaVersion = 1
type Registry struct {
	SchemaVersion int
	UpdatedAt     time.Time
	Profiles      []RegistryEntry
} // JSON tag 不变
type RegistryEntry struct{ Name, Path, SHA256 string }
func DecodeRegistry(data []byte) (Registry, error)                              // 现有严格规则
func EncodeRegistry(registry Registry) ([]byte, error)                          // 新增，见下
func ValidateRegistryPath(path string) error
func ValidateRegistryProfile(entry RegistryEntry, data []byte) (Overlay, error) // SHA256 + DecodeOverlay + name 一致
func ProfileSHA256(data []byte) string                                          // 小写 hex，无前缀
func ContentAddressedProfilePath(name, sha256Hex string) (string, error)        // profiles/<name>/<sha256>.json
func ResolveRegistryURL(baseURL, path string) (string, error)
type RegistryFetcher struct {
	HTTPClient *http.Client // nil 时使用不读代理环境的直连客户端，超时 30 秒
	BaseURL    string       // 绝对 http(s) URL，以 "/" 结尾
	NoCache    bool         // 发送 Cache-Control: no-cache
}
type FetchedRegistry struct {
	IndexBytes  []byte
	IndexSHA256 string
	Registry    Registry
	Profiles    []FetchedProfile // 与 index 顺序一致
}
type FetchedProfile struct {
	Entry   RegistryEntry
	URL     string
	Bytes   []byte
	Overlay Overlay
}
func (f RegistryFetcher) Fetch(ctx context.Context) (FetchedRegistry, error)
func (r FetchedRegistry) Profile(name string) (FetchedProfile, bool)

// Rebase
type RebaseResult, RebaseFieldChange, RebaseUnresolved
func RebaseOverlay(previous Overlay, template Template) (RebaseResult, error)
```

允许为实现增加未导出的辅助函数；新增导出符号需要有充分理由并写进 CHANGELOG。

## 新增能力规格

### DecodeOverlayDraft

与 `DecodeOverlay` 共用同一套 wire 解码（未知字段、缺失必需属性、null 可选属性、trailing JSON、
fingerprint 格式都照旧拒绝），只是不调用 `ValidateOverlaySchema`。它用于持久化尚未补全 routing 的草稿，
必须保留 baseline 表格字段 `columns` 的存在性，使 `DecodeOverlayDraft(json.Marshal(o))` 后再
`ValidateOverlaySchema` 的结论与原值一致。`DecodeOverlay` 改为 `DecodeOverlayDraft` + `ValidateOverlaySchema`。

### EncodeProfile

1. `ValidateOverlaySchema(overlay)` 失败即返回错误。
2. 在副本上把 `Routing.Examples`、`Baseline.Fields` 的 nil 归一为空切片，不修改调用方的值。
3. `json.MarshalIndent(copy, "", "  ")` 后追加一个 `\n`。
4. 回读自检：`DecodeOverlay(bytes)` 必须成功，并且对解码结果再次执行第 2、3 步得到完全相同的字节；否则返回错误。
5. 与 `ixf servicedesk profile discover` 的现有输出逐字节兼容：对任何由 discover 写出的文件 `b`，
   `EncodeProfile(DecodeOverlay(b)) == b`。用合成 fixture 测试，并在本地对线上 profile 做一次不入库的核对。

### EncodeRegistry

1. `SchemaVersion` 必须为 1；`UpdatedAt` 不能为零值，编码为 `UTC().Truncate(time.Second)` 的 RFC3339。
2. 每个 entry 满足 `DecodeRegistry` 的全部规则：合法 name、唯一 name、`ValidateRegistryPath`、64 位小写 hex、
   路径之间以及与 `index.json` 之间无文件前缀冲突。
3. 在副本上按 name 排序；空列表编码为 `[]`。
4. 键顺序 `schemaVersion`、`updatedAt`、`profiles`；entry 键顺序 `name`、`path`、`sha256`；
   两空格缩进，末尾一个 `\n`。
5. 回读自检：`DecodeRegistry(bytes)` 成功且再次编码字节相同。与现有线上 index 的格式逐字节兼容（本地核对，不入库）。

### ContentAddressedProfilePath 与 ResolveRegistryURL

- `ContentAddressedProfilePath` 校验 `IsValidProfileName(name)` 和 `^[0-9a-f]{64}$`，返回
  `profiles/<name>/<sha256>.json`，结果必须通过 `ValidateRegistryPath`。
- `ResolveRegistryURL` 要求 base 是绝对 `http`/`https` URL、无 userinfo/query/fragment、路径以 `/` 结尾；
  path 先经 `ValidateRegistryPath`；解析结果必须与 base 同 scheme、同 host，且路径以 base 路径为前缀。
  现有 `registryProfileURL` 改为调用它。

### RegistryFetcher

`Fetch` 读取 `BaseURL + "index.json"`，按 `DecodeRegistry` 校验，再按 index 顺序读取每个 entry 并经
`ValidateRegistryProfile` 校验。任何一个 entry 失败都返回错误（与客户端现有“整个 Registry 失败”语义一致）。
HTTP 行为与现有 `RegistryLoader.fetch` 一致：克隆客户端、拒绝重定向、只接受 2xx、单个响应 8 MiB 上限、
默认不读代理环境。`NoCache` 为 true 时每个请求带 `Cache-Control: no-cache`。
`internal/servicedesk.RegistryLoader` 的网络路径改用 `RegistryFetcher{HTTPClient: l.HTTPClient, BaseURL: DefaultRegistryURL}`，
缓存读写逻辑留在内部包，现有 registry 测试全部保持通过。

### RebaseOverlay

把已发布 profile 迁移到当前实时表单。输入 `previous` 必须通过 `ValidateOverlaySchema`；
`template.Target` 必须等于 `previous.Target`，否则返回错误。结果必须确定（相同输入得到 deep-equal 的结果，
所有列表排序），并且 `ValidateOverlaySchema(result.Overlay)` 与 `ResolveOverlay(result.Overlay, template)`
都不报错。

```go
type RebaseResult struct {
	Overlay    Overlay             `json:"overlay"`
	Preserved  []RebaseFieldChange `json:"preserved"`
	Added      []RebaseFieldChange `json:"added"`
	Removed    []RebaseFieldChange `json:"removed"`
	Unresolved []RebaseUnresolved  `json:"unresolved"`
}
type RebaseFieldChange struct {
	Field      string   `json:"field"`
	Column     string   `json:"column,omitempty"`
	MatchedBy  string   `json:"matchedBy,omitempty"`  // stableId | key | name
	Properties []string `json:"properties,omitempty"` // 迁移了的定制属性
}
type RebaseUnresolved struct {
	ID         string          `json:"id"`     // unresolved-1, unresolved-2 ... 按排序后的位置编号
	Kind       string          `json:"kind"`   // 见下
	Field      string          `json:"field"`  // previous 中的公共字段名
	Column     string          `json:"column,omitempty"`
	Properties []string        `json:"properties"` // 未迁移的定制属性
	Previous   json.RawMessage `json:"previous"`   // 原 OverlayField/OverlayColumn JSON；default_invalid 时为 {"default": ...}
	Message    string          `json:"message"`
}
```

规则：

1. `Name`、`Description`、`Routing`（深拷贝）、`Target`、`SchemaVersion` 来自 `previous`；baseline 与
   字段清单来自 `DiscoverOverlay(template, previous.Name)`。
2. 属性名取值：`publicName`、`aliases`、`description`、`default`、`required`、`columns`。
   字段“已定制”指：公共名不等于其 previous baseline 字段的 `stableId`（为空时用 `name`），或有 aliases、
   description、default、`required: true`，或任一列已定制。列“已定制”指：公共名不等于 previous baseline 列的
   `name`，或有 aliases、description、default、`required: true`。`required: false` 覆盖与缺省语义等价，
   不视为定制，也不迁移。
3. previous 的每个公共字段先用其 matcher 解析到 previous baseline 字段，再用该 baseline 字段的
   `stableId`/`name` 以与 drift 相同的 matcher 规则匹配实时字段：stableId 唯一优先，其次唯一名称回退。
4. 多个 previous 字段落到同一实时字段时，唯一一个通过 stableId 命中的保留；其余（以及没有 stableId 命中者时
   的全部竞争者）视为 `field_ambiguous`。
5. 命中且类型完全相同：保留 previous 公共名，matcher 取自实时字段，迁移 aliases、description、default、
   `required: true`。表格字段的列按同样规则处理（key 唯一优先、唯一名称回退、key 命中者在竞争中胜出、
   类型完全相同才迁移），种类为 `column_missing`、`column_ambiguous`、`column_type_changed`。
6. 未命中、歧义或类型变化：已定制的进入 `Unresolved`（`field_missing`、`field_ambiguous`、
   `field_type_changed` 或对应 column 种类），不迁移任何配置；未定制的进入 `Removed`。
7. 迁移的 default 必须对实时字段有效：非表格字段用与 `ResolveOverlay` 相同的 live default 校验；
   表格字段级 default 行用新列映射做与 `ResolveOverlay` 相同的行校验；列 default 同理。
   无效时丢弃该 default，记录 `default_invalid`。
8. 未被 previous 认领的实时字段和列按 discover 规则生成公共名，且不得与已保留的公共名和 aliases 冲突
   （先登记保留名和 aliases，再调用与 discover 相同的唯一化规则）。这些条目进入 `Added`。
9. `Unresolved` 按 `(field, column, kind)` 排序后编号；`Preserved`、`Added`、`Removed` 按 `(field, column)` 排序。
10. 消息为英文短句，不包含后端 lading 字段 key 或实时默认值。

至少覆盖的测试：无变化 rebase 与原 profile `EncodeProfile` 字节相同；snapshot 变化和显示名改名时定制被保留；
新增可选字段及其与保留 alias 的冲突；已定制字段被删除进入 Unresolved、未定制进入 Removed；类型变化；
stableId 变化但名称唯一的回退；重复名称歧义；表格列的 key 命中、名称回退、删除和新增；选项变化使 select default
失效；表格 default 行引用被删列；`required: false` 被丢弃；确定性（多次执行和大量字段下结果一致）；
target 不一致与 previous 非法时报错。

## 其他测试要求

- `pkg/servicedeskprofile` 增加导入白名单测试：用 `go/parser` 读取包内非测试文件，断言只导入标准库和
  `internal/servicedeskapi`；同样断言 `internal/servicedeskapi` 不导入 `pkg/...` 和 `internal/servicedesk`。
- 反射断言 `*MetadataClient` 的导出方法集恰好是 `Lading`、`Service`。
- `EncodeProfile`/`EncodeRegistry` 的确定性和回读自检失败路径。
- `RegistryFetcher`：重定向拒绝、非 2xx、超大响应、任一 entry SHA256 不符、`NoCache` 头。

## 提交顺序

1. `refactor: extract ServiceDesk profile contract package`（纯迁移，无行为变化）
2. `feat: add ServiceDesk profile encoding and registry helpers`
3. `feat: add deterministic ServiceDesk profile rebase`
4. `docs: record ServiceDesk profile package`（CHANGELOG `## Unreleased`、包文档注释）

版本号与发布由维护者单独进行（`chore/release-*` 流程）。外部模块的路径不带 `/v3`，因此 Go 无法直接
`require v3.x.y` 标签；Maintainer 需要固定到发布提交的伪版本，例如
`go get github.com/serialq7ic4/ixf-toolbox@<release-commit>`。

---

### Task 1: 抽取元数据传输层和 profile 领域包

**Files:**
- Create: `internal/servicedeskapi/client.go`
- Create: `internal/servicedeskapi/client_test.go`
- Create: `internal/servicedeskapi/url.go`
- Create: `internal/servicedeskapi/url_test.go`
- Create: `pkg/servicedeskprofile/doc.go`
- Create: `pkg/servicedeskprofile/{url,metadata,overlay,drift,contract}.go`
- Move tests into: `pkg/servicedeskprofile/{url,metadata,overlay,drift,contract}_test.go`
- Modify: `internal/servicedesk/{client,url,metadata,overlay,drift,contract,create,registry}.go`
- Modify: `internal/servicedesk/*_test.go`, `cmd/ixf/servicedesk.go`

**Interfaces:**
- Produces: 本计划“公共 API”中 URL、MetadataSource、MetadataClient、Template、Overlay、drift 和 contract 符号。
- Preserves: `internal/servicedesk.Client` 的现有完整工单 API，并通过委托满足 `servicedeskprofile.MetadataSource`。

- [ ] **Step 1: 写 package 边界和行为兼容失败测试**

新增测试：

- `TestPublicPackageImportsStayPure`：非测试文件只能导入标准库和 `internal/servicedeskapi`。
- `TestMetadataClientExportedMethodSet`：导出方法恰好是 `Lading`、`Service`。
- `TestDecodeOverlayDraftPreservesStrictWireRules`：未知字段、null、trailing JSON 和非法 fingerprint 失败；未补 routing 的合法草稿可解码。
- 原有 `TestInspect...`、`TestValidateOverlay...`、`TestParseEditURL...` 全部迁移后断言不变。

- [ ] **Step 2: 运行定向测试并确认 RED**

Run: `go test ./pkg/servicedeskprofile ./internal/servicedeskapi ./internal/servicedesk`

Expected: FAIL，因为新 package 和导出接口尚不存在。

- [ ] **Step 3: 实现纯传输层和领域代码迁移**

按“目标布局”和“公共 API”逐个迁移；`DecodeOverlay` 实现为 `DecodeOverlayDraft` 后调用
`ValidateOverlaySchema`。内部 package 直接使用 `sdp` 限定名，不保留类型别名。

- [ ] **Step 4: 运行定向及全量测试并确认 GREEN**

Run: `go test ./pkg/servicedeskprofile ./internal/servicedeskapi ./internal/servicedesk ./cmd/ixf`

Expected: PASS，且 `rg -n '^func Test' internal/servicedesk pkg/servicedeskprofile cmd/ixf/servicedesk_test.go | wc -l` 不少于迁移前的 160。

- [ ] **Step 5: 执行阶段验证**

Run: `go test ./... && go vet ./... && go run ./cmd/pluginpack --check`

Expected: 全部退出 0。

- [ ] **Step 6: 提交**

```bash
git add internal/servicedeskapi pkg/servicedeskprofile internal/servicedesk cmd/ixf
git commit -m "refactor: extract ServiceDesk profile contract package"
```

### Task 2: 增加确定性编码和 Registry 工具

**Files:**
- Create: `pkg/servicedeskprofile/encoding.go`
- Create: `pkg/servicedeskprofile/encoding_test.go`
- Create: `pkg/servicedeskprofile/registry.go`
- Create: `pkg/servicedeskprofile/registry_test.go`
- Modify: `internal/servicedesk/registry.go`
- Modify: `internal/servicedesk/registry_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Overlay`、`Registry`、`MetadataClient` 和严格 decoder。
- Produces: `EncodeProfile`、`EncodeRegistry`、`ProfileSHA256`、`ContentAddressedProfilePath`、`ResolveRegistryURL`、`RegistryFetcher.Fetch`。

- [ ] **Step 1: 写编码、路径和 HTTP 边界失败测试**

覆盖本计划“EncodeProfile”“EncodeRegistry”“ContentAddressedProfilePath”“RegistryFetcher”中的全部规则，特别包括重复编码字节一致、重定向拒绝、非 2xx、8 MiB 上限、SHA256 不符、`NoCache` 和代理环境不生效。

- [ ] **Step 2: 运行测试并确认 RED**

Run: `go test ./pkg/servicedeskprofile ./internal/servicedesk -run 'Encode|Registry|ContentAddressed|ResolveRegistryURL'`

Expected: FAIL，因为导出 helper 和 fetcher 尚不存在。

- [ ] **Step 3: 实现确定性编码和 Registry fetcher**

严格按“新增能力规格”实现；内部 `RegistryLoader` 网络路径改用公共 fetcher，缓存落盘和 stale 规则仍留在内部包。

- [ ] **Step 4: 运行定向和全量测试并确认 GREEN**

Run: `go test ./pkg/servicedeskprofile ./internal/servicedesk && go test ./...`

Expected: PASS。

- [ ] **Step 5: 本地兼容性核对，不写入仓库**

用临时 Go 程序读取当前 43 的 `index.json` 和 profile，断言 decode/encode 逐字节一致；临时文件退出后删除。

Expected: 输出 `index compatible` 和 `profile compatible`，git 状态不包含线上 fixture。

- [ ] **Step 6: 提交**

```bash
git add pkg/servicedeskprofile internal/servicedesk
git commit -m "feat: add ServiceDesk profile encoding and registry helpers"
```

### Task 3: 增加确定性 profile rebase

**Files:**
- Create: `pkg/servicedeskprofile/rebase.go`
- Create: `pkg/servicedeskprofile/rebase_test.go`
- Modify: `pkg/servicedeskprofile/overlay.go`
- Modify: `pkg/servicedeskprofile/drift.go`

**Interfaces:**
- Consumes: Task 1 的 discover/resolve/default 校验和 Task 2 的确定性编码。
- Produces: `RebaseOverlay(previous Overlay, template Template) (RebaseResult, error)` 及计划定义的结果类型。

- [ ] **Step 1: 写 rebase 失败测试**

按“至少覆盖的测试”逐项添加 table-driven tests；必须显式断言字段/列缺失、歧义、类型变化和无效 default 进入 `Unresolved`，不被静默丢弃。

- [ ] **Step 2: 运行测试并确认 RED**

Run: `go test ./pkg/servicedeskprofile -run Rebase`

Expected: FAIL，因为 `RebaseOverlay` 尚不存在。

- [ ] **Step 3: 实现 rebase**

按照本计划十条规则实现稳定匹配、定制迁移、冲突保留、unresolved 编号和排序；共享现有 default 校验 helper，不复制条件或表格校验逻辑。

- [ ] **Step 4: 运行定向和 race 测试并确认 GREEN**

Run: `go test ./pkg/servicedeskprofile -run Rebase && go test -race ./pkg/servicedeskprofile`

Expected: PASS。

- [ ] **Step 5: 执行全量阶段验证**

Run: `go test ./... && go vet ./... && go run ./cmd/pluginpack --check`

Expected: 全部退出 0。

- [ ] **Step 6: 提交**

```bash
git add pkg/servicedeskprofile
git commit -m "feat: add deterministic ServiceDesk profile rebase"
```

### Task 4: 文档、构建和最终兼容验证

**Files:**
- Create: `pkg/servicedeskprofile/README.md`
- Modify: `CHANGELOG.md`
- Modify: `docs/go-python-parity.md`
- Modify: `docs/superpowers/plans/2026-11-24-servicedesk-profile-package.md`

**Interfaces:**
- Consumes: Tasks 1-3 的最终公共 API。
- Produces: 外部 Maintainer 可依赖的包文档和验证记录。

- [ ] **Step 1: 写包文档和 Unreleased 说明**

文档明确纯读取元数据边界、公开符号用途、无环境副作用、外部模块固定 commit/pseudo-version 的方式，以及真实工单写操作不在公共包中。

- [ ] **Step 2: 执行完整验证**

Run:

```bash
go test ./...
go test -race ./pkg/servicedeskprofile ./internal/servicedeskapi ./internal/servicedesk
go vet ./...
go run ./cmd/pluginpack --check
CGO_ENABLED=0 go build -trimpath -o /tmp/ixf-go ./cmd/ixf
scripts/smoke-go-binary.sh /tmp/ixf-go "$(cat VERSION)"
```

Expected: 全部退出 0；`git diff --check` 无输出。

- [ ] **Step 3: 提交**

```bash
git add pkg/servicedeskprofile/README.md CHANGELOG.md docs/go-python-parity.md docs/superpowers/plans/2026-11-24-servicedesk-profile-package.md
git commit -m "docs: record ServiceDesk profile package"
```
