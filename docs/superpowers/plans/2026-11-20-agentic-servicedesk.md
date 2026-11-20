# Agentic ServiceDesk Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an Agent-first, Go-native, API-only ServiceDesk capability that discovers live forms, validates structured inputs, safely creates and withdraws tickets, and lists the current user's tickets.

**Architecture:** A focused `internal/servicedesk` package owns credentials, fixed-host HTTP, live metadata, overlay/Registry resolution, input normalization, write preflight, and ticket lifecycle operations. `cmd/ixf/servicedesk.go` exposes the stable versioned JSON contract, while canonical Codex/Claude skills provide semantic intent and profile routing without duplicating protocol logic.

**Tech Stack:** Go standard library (`net/http`, `encoding/json`, `crypto/sha256`, `net/url`, `os`), repository CLI/pluginpack conventions, `httptest`, JSON fixtures, ServiceDesk internal HTTP API.

**Spec:** `docs/superpowers/specs/2026-11-20-servicedesk-api-profile-design.md`

## Global Constraints

- Runtime is Go-only; do not add Python, browser automation, Cookie auth, MCP, or a central ticket proxy.
- Production requests accept only `https://servicedesk.iflytek.com`; tests inject an `http.Client`/transport rather than exposing a base-URL flag.
- Never print, persist in the repository, snapshot, or pass through command-line values any API Key, Cookie, auth header, or raw userinfo response.
- API Key precedence is `IXF_SERVICEDESK_API_KEY`, then `--api-key-file`, then `~/.config/ixf-toolbox/servicedesk-api-key`; there is no `--api-key` flag.
- All Agent-facing commands support `--json` and emit `contractVersion: 1`; usage errors exit 2 and domain failures emit one JSON document plus a nonzero exit.
- `create` and `withdraw` default to dry-run, require explicit `--apply`, rerun preflight at apply time, and verify writes by readback.
- A timed-out create POST is never retried automatically and returns `outcome=unknown` with safe reconciliation data.
- Live ServiceDesk metadata is authoritative; Registry stores semantic overlay only, with a 24-hour apply freshness limit.
- The CLI never performs keyword scoring, natural-language profile resolution, ticket semantic matching, or business-fact invention; those decisions remain with the Agent.
- Only `request` service keys are writable in v1; unsupported required fields, unknown consequential conditions, destructive drift, and unknown withdraw states fail closed.
- Registry defaults are `http://10.103.251.43/files/ixf-toolbox/servicedesk/v1/` and `/disk1/files/ixf-toolbox/servicedesk/v1/`.
- Production code and committed tests must not contain the user's API Key or other live credentials.
- This feature release changes `VERSION` from `3.31.3` to `3.32.0`; generated plugin packages must match that version before merge.

## Review Focus

- A create POST whose response is lost after the server accepts it must produce `outcome=unknown`, make exactly one POST, and retain enough safe title/time/target context for manual list-based reconciliation.
- Duplicate display names, aliases, or choice labels must never resolve by first match; the error identifies the ambiguous public field or table column and blocks apply.
- Registry network failure must distinguish valid fresh cache, stale cache, corrupt/hash-mismatched cache, and no cache; stale data can describe/dry-run but cannot apply.
- Multi-scope ticket listing must deduplicate aggregate and dedicated business results, preserve descending creation order, and return `complete=false` with failed scopes instead of claiming completeness.
- API Key values must stay absent from successful JSON, typed API errors, malformed-response errors, doctor output, stderr, and test snapshots even when the server echoes request data.

---

### Task 1: Fixed-Host Client, Credentials, URLs, and Sanitized Fixtures

**Files:**
- Create: `internal/servicedesk/client.go`
- Create: `internal/servicedesk/auth.go`
- Create: `internal/servicedesk/url.go`
- Create: `internal/servicedesk/client_test.go`
- Create: `internal/servicedesk/auth_test.go`
- Create: `internal/servicedesk/url_test.go`
- Create: `internal/servicedesk/testdata/userinfo.json`
- Create: `internal/servicedesk/testdata/service-4003.json`
- Create: `internal/servicedesk/testdata/lading-4003.json`
- Create: `internal/servicedesk/testdata/list.json`
- Create: `internal/servicedesk/testdata/start.json`
- Create: `internal/servicedesk/testdata/ticket-running.json`
- Create: `internal/servicedesk/testdata/ticket-withdrawn.json`
- Create: `internal/servicedesk/testdata/withdraw.json`

**Interfaces:**
- Consumes: ServiceDesk envelope and endpoint shapes documented in the approved spec; Go `http.Client`, `io/fs`, and clock injection.
- Produces: `Client`, `ClientOptions`, `APIError`, `CredentialLoader`, `Credential`, `BusinessIDLoader`, `EditTarget`, `DetailTarget`, `ParseEditURL(string)`, and `ParseDetailURL(string)` for every later task.

- [ ] **Step 1: Add sanitized protocol fixtures**

Store the verified response shapes for userinfo, service, lading, list, start, get, and withdraw. Preserve real structural keys needed by parsers, replace people/hosts/private text with deterministic test values, and assert in `client_test.go` that no fixture contains the live API Key, `Cookie`, or `apikey` values.

- [ ] **Step 2: Write failing credential and URL tests**

Add table tests named `TestLoadCredentialPrecedence`, `TestLoadCredentialRejectsEmptyAndUnsafeFiles`, `TestLoadBusinessIDPrecedence`, `TestParseEditURL`, and `TestParseDetailURL`. Assert environment-over-file precedence, Unix mode `0600` enforcement, positive business IDs, exact host/scheme/hash paths, required IDs, and rejection of alternate hosts, malformed numbers, query-only edit paths, and extra endpoint data.

- [ ] **Step 3: Run the focused tests and confirm failure**

Run: `go test ./internal/servicedesk -run 'Test(Load|Parse)' -count=1`

Expected: FAIL because the package interfaces do not exist.

- [ ] **Step 4: Implement credentials and URL parsing**

Implement:

```go
type Credential struct { APIKey, Source string }
type CredentialLoader struct { LookupEnv func(string) (string, bool); ReadFile func(string) ([]byte, error); Stat func(string) (fs.FileInfo, error); GOOS, HomeDir string }
func (l CredentialLoader) Load(explicitFile string) (Credential, error)

type BusinessIDLoader struct { LookupEnv func(string) (string, bool); ReadFile func(string) ([]byte, error); HomeDir string }
func (l BusinessIDLoader) Load(explicit int) (id int, source string, configured bool, err error)

type EditTarget struct { BusinessID, ServiceID int }
type DetailTarget struct { BusinessID int; TicketID string }
func ParseEditURL(raw string) (EditTarget, error)
func ParseDetailURL(raw string) (DetailTarget, error)
```

Trim only credential edge whitespace, validate nonempty values without assuming key length/charset, and never include credential content in errors.

- [ ] **Step 5: Write failing HTTP client tests**

Cover required `businessId`/`apikey`/JSON headers, endpoint paths and queries, HTTP 401 versus 403 classification, HTTP 200 with `result=false`, malformed envelopes, timeout identity, response-size limits, and redaction when a synthetic server echoes the API Key. Add `TestClientNeverRetriesCreateTimeout` asserting one `start` request.

- [ ] **Step 6: Implement the fixed-host HTTP client**

Implement:

```go
type ClientOptions struct { HTTPClient *http.Client; Transport http.RoundTripper; Now func() time.Time }
type Client struct { /* unexported fixed-origin state */ }
func NewClient(credential Credential, options ClientOptions) *Client
type APIError struct { Code string; Status int; Retryable bool; Message string; Cause error }
func (e *APIError) Error() string
func (c *Client) UserInfo(ctx context.Context, businessID int) (UserInfo, error)
func (c *Client) Service(ctx context.Context, target EditTarget) (Service, error)
func (c *Client) Lading(ctx context.Context, target EditTarget, snapshotID string) (Lading, error)
func (c *Client) Start(ctx context.Context, businessID int, payload StartPayload) (StartResponse, error)
func (c *Client) List(ctx context.Context, businessID int, query ListQuery) (TicketPage, error)
func (c *Client) Get(ctx context.Context, target DetailTarget) (TicketResponse, error)
func (c *Client) Withdraw(ctx context.Context, businessID int, payload WithdrawPayload) error
```

Add typed methods for `UserInfo`, `Service`, `Lading`, `Start`, `List`, `Get`, and `Withdraw`; all production URLs originate from a private ServiceDesk constant, while tests replace transport routing without changing the request host.

- [ ] **Step 7: Verify Task 1**

Run: `go test ./internal/servicedesk -run 'Test(Client|Load|Parse)' -count=1`

Expected: PASS; request-count assertion proves no create retry and fixture privacy assertion passes.

- [ ] **Step 8: Commit Task 1**

```bash
git add internal/servicedesk
git commit -m "feat: add ServiceDesk API foundations"
```

### Task 2: Live Metadata and Resolved Agent Contract

**Files:**
- Create: `internal/servicedesk/metadata.go`
- Create: `internal/servicedesk/metadata_test.go`
- Create: `internal/servicedesk/contract.go`
- Create: `internal/servicedesk/contract_test.go`

**Interfaces:**
- Consumes: `Client.Service`, `Client.Lading`, `EditTarget`, and Task 1 fixtures.
- Produces: normalized `Template`, `Field`, `Choice`, `TableColumn`, `ConditionSet`, `InspectRequest`, `ResolvedContract`, `Inspect(context.Context, InspectRequest)`, and stable metadata/contract hashes.

- [ ] **Step 1: Write failing metadata normalization tests**

Add `TestNormalizeVerifiedLadingFixture`, `TestNormalizeNestedTableAndChoices`, `TestNormalizeDefaultsAndDisplayOnlyFields`, and `TestNormalizeUnknownRequiredType`. Assert backend key, stable ID, display name, field type, required/default presence, exact choice label/value pairs, nested column fingerprints, condition source, and unsupported-required count.

- [ ] **Step 2: Run tests and confirm failure**

Run: `go test ./internal/servicedesk -run 'TestNormalize' -count=1`

Expected: FAIL because metadata normalization is undefined.

- [ ] **Step 3: Implement normalized live metadata**

Implement:

```go
type Choice struct { Label, Value string }
type TableColumn struct { BackendKey, Name, Type string; Required bool; Choices []Choice }
type Field struct { BackendKey, StableID, Name, Type string; Required bool; DefaultValue any; DefaultAvailable bool; Choices []Choice; Conditions ConditionSet; TableColumns []TableColumn; Supported bool }
type Template struct { Target EditTarget; SnapshotID, ServiceKey, ServiceName string; Fields []Field; MetadataHash string }
func LoadTemplate(ctx context.Context, client *Client, target EditTarget) (Template, error)
```

Use canonical JSON with sorted map-derived collections for `sha256:` hashes; preserve unknown types as inspectable `Supported=false` rather than discarding them.

- [ ] **Step 4: Write failing resolved-contract tests**

Add `TestInspectURLResolvedContract`, `TestResolvedContractIncludesChoicesTablesAndConditions`, `TestResolvedContractRejectsDuplicateDisplayName`, `TestContractHashIsStable`, and `TestInspectReportsUnsupportedRequiredField`. Assert `contractVersion=1`, public URL-mode names prefer `StableID` and otherwise unique display names, complete choice/table descriptions, unsupported counts, and deterministic hash across map order.

- [ ] **Step 5: Implement the URL-mode Agent contract**

Implement:

```go
type InspectRequest struct { Target EditTarget; Overlay *Overlay; RegistryStatus *RegistryStatus }
type ResolvedContract struct { Target ResolvedTarget; Profile *ResolvedProfile; InputContract InputContract; TemplateDrift bool; DriftClassification string; DriftChanges []DriftChange; ContractHash string }
func Inspect(ctx context.Context, client *Client, request InspectRequest) (ResolvedContract, error)
```

Keep public contract fields free of backend payloads, API Keys, and raw metadata. Define profile-dependent types now with zero-value behavior; Task 4 fills overlay resolution.

- [ ] **Step 6: Verify Task 2**

Run: `go test ./internal/servicedesk -run 'Test(Normalize|Inspect|Resolved|Contract)' -count=1`

Expected: PASS with stable hash assertions.

- [ ] **Step 7: Commit Task 2**

```bash
git add internal/servicedesk/metadata.go internal/servicedesk/metadata_test.go internal/servicedesk/contract.go internal/servicedesk/contract_test.go
git commit -m "feat: resolve live ServiceDesk form contracts"
```

### Task 3: Strict Overlay Schema, Discovery, Matching, and Drift

**Files:**
- Create: `internal/servicedesk/overlay.go`
- Create: `internal/servicedesk/overlay_test.go`
- Create: `internal/servicedesk/drift.go`
- Create: `internal/servicedesk/drift_test.go`
- Create: `internal/servicedesk/testdata/profile-4003.json`

**Interfaces:**
- Consumes: `Template`, normalized fields/columns, canonical hashing from Task 2.
- Produces: `Overlay`, strict `DecodeOverlay`, `DiscoverOverlay`, `ResolveOverlay`, `ValidateOverlay`, `DriftReport`, and public-name lookup maps consumed by Registry, inspect, and create.

- [ ] **Step 1: Write failing strict-schema tests**

Add `TestDecodeOverlayStrictSchema`, `TestOverlayNameAndRoutingValidation`, `TestOverlayRejectsDuplicatePublicNamesAndAliases`, `TestOverlayRejectsExecutableOrNetworkFields`, and `TestOverlayRequiredCannotRelaxServiceRequired`. Include unknown properties at each nesting level, invalid slugs, duplicate aliases, empty/oversized routing text, invalid matcher objects, unsupported baseline shapes, and an overlay default with the wrong public JSON type.

- [ ] **Step 2: Run schema tests and confirm failure**

Run: `go test ./internal/servicedesk -run 'Test(DecodeOverlay|Overlay)' -count=1`

Expected: FAIL because overlay decoding is undefined.

- [ ] **Step 3: Implement strict overlay decoding and validation**

Implement the schema exactly as the spec using `json.Decoder.DisallowUnknownFields` at every typed level:

```go
func DecodeOverlay(data []byte) (Overlay, error)
func ValidateOverlaySchema(overlay Overlay) error
func DiscoverOverlay(template Template, name string) Overlay
func ValidateOverlay(template Template, overlay Overlay) (ResolvedOverlay, error)
```

`DiscoverOverlay` includes all top-level baseline fingerprints and deterministic default public names but adds no business defaults, required strengthening, or inferred routing semantics.

- [ ] **Step 4: Write failing matcher and drift tests**

Add `TestResolveOverlayStableIDThenUniqueName`, `TestResolveOverlayColumnKeyThenUniqueName`, `TestResolveOverlayRejectsAmbiguity`, `TestValidateOverlayUsesLiveTemplate`, `TestDriftClassifiesSnapshotOnlyAndOptionalFieldAsNonDestructive`, and `TestDriftClassifiesMissingMatcherTypeChoiceRequiredAndConditionChangesAsDestructive`. Assert fallback resolution records drift, online validation checks the current template rather than baseline alone, and no matcher selects the first duplicate.

- [ ] **Step 5: Implement overlay resolution and drift classification**

Implement:

```go
type ResolvedOverlay struct { Overlay Overlay; Fields map[string]ResolvedField; Aliases map[string]string; Drift DriftReport }
type DriftReport struct { Changed bool; Classification string; Changes []DriftChange; ApplyBlocked bool }
func ResolveOverlay(overlay Overlay, template Template) (ResolvedOverlay, error)
func CompareBaseline(baseline Baseline, template Template) DriftReport
```

Use stable ID/key first and unique-name fallback second. Resolve all fields and columns before returning, accumulating structured drift changes instead of hiding later errors.

- [ ] **Step 6: Integrate profile-mode inspect**

Update `Inspect` to use resolved public field names, aliases, profile source/staleness, descriptions/default availability, and drift. Add `TestInspectProfileResolvedContract` and verify a destructive drift sets the contract's apply block metadata.

- [ ] **Step 7: Verify Task 3**

Run: `go test ./internal/servicedesk -run 'Test(DecodeOverlay|Overlay|ResolveOverlay|Drift|InspectProfile)' -count=1`

Expected: PASS with ambiguity and destructive-drift cases failing closed.

- [ ] **Step 8: Commit Task 3**

```bash
git add internal/servicedesk/overlay.go internal/servicedesk/overlay_test.go internal/servicedesk/drift.go internal/servicedesk/drift_test.go internal/servicedesk/contract.go internal/servicedesk/contract_test.go internal/servicedesk/testdata/profile-4003.json
git commit -m "feat: add ServiceDesk profile overlays and drift checks"
```

### Task 4: Registry Download, Integrity, and Cache Policy

**Files:**
- Create: `internal/servicedesk/registry.go`
- Create: `internal/servicedesk/registry_test.go`

**Interfaces:**
- Consumes: `DecodeOverlay`, clock injection, HTTP client, and cache filesystem.
- Produces: `Registry`, `RegistryEntry`, `RegistryStatus`, `RegistryLoader.List`, `RegistryLoader.Show`, and fresh/stale policy consumed by inspect/create and CLI profile commands.

- [ ] **Step 1: Write failing Registry tests**

Add `TestRegistryLoadsAndVerifiesNetworkIndex`, `TestRegistryListReturnsRoutingSummaryWithoutDefaultsOrBaseline`, `TestRegistryRejectsUnknownSchemaDuplicateNameAndPathTraversal`, `TestRegistryRejectsHashOrOverlayNameMismatch`, `TestRegistryFallsBackToFreshCache`, `TestRegistryStaleCacheReadAndDryRunAllowedApplyBlocked`, and `TestRegistryCorruptCacheIsNotUsed`. Assert clean relative slash paths, no query/fragment/backslash/absolute traversal, RFC3339 `updatedAt`, exact lowercase SHA-256, source/fetchedAt/stale fields, the 24-hour boundary, and list output limited to name/description/routing/target.

- [ ] **Step 2: Run Registry tests and confirm failure**

Run: `go test ./internal/servicedesk -run TestRegistry -count=1`

Expected: FAIL because `RegistryLoader` is undefined.

- [ ] **Step 3: Implement Registry and cache loading**

Implement:

```go
const DefaultRegistryURL = "http://10.103.251.43/files/ixf-toolbox/servicedesk/v1/"
type RegistryStatus struct { Source string; FetchedAt time.Time; Stale bool }
type RegistryLoader struct { HTTPClient *http.Client; CacheDir string; Now func() time.Time }
type ProfileSummary struct { Name, Description string; Routing Routing; Target OverlayTarget }
func (l RegistryLoader) List(ctx context.Context) ([]ProfileSummary, RegistryStatus, error)
func (l RegistryLoader) Show(ctx context.Context, name string) (Overlay, RegistryStatus, error)
func (s RegistryStatus) ApplyAllowed() bool
```

Write cache files atomically only after complete index/schema/hash/overlay validation. Do not cache failed or partially validated network responses.

- [ ] **Step 4: Verify Task 4**

Run: `go test ./internal/servicedesk -run TestRegistry -count=1`

Expected: PASS for network, cache, stale, integrity, and traversal cases.

- [ ] **Step 5: Commit Task 4**

```bash
git add internal/servicedesk/registry.go internal/servicedesk/registry_test.go
git commit -m "feat: add ServiceDesk profile registry cache"
```

### Task 5: Input Types, Conditions, and Create Dry-Run

**Files:**
- Create: `internal/servicedesk/fields.go`
- Create: `internal/servicedesk/fields_test.go`
- Create: `internal/servicedesk/conditions.go`
- Create: `internal/servicedesk/conditions_test.go`
- Create: `internal/servicedesk/create.go`
- Create: `internal/servicedesk/create_test.go`

**Interfaces:**
- Consumes: live `Template`, optional `ResolvedOverlay`, Registry freshness, Task 1 client/auth, and strict input JSON.
- Produces: `DecodeTicketInput`, `NormalizeInput`, `EvaluateConditions`, `CreateRequest`, `CreatePlan`, `CreateResult`, and `Create` used by CLI and apply.

- [ ] **Step 1: Write failing strict input and scalar-type tests**

Add `TestDecodeTicketInputDirectAndFieldsWrapper`, `TestDecodeTicketInputRejectsUnknownWrapperProperties`, `TestDecodeTicketInputRejectsDuplicateJSONKeys`, and a table-driven `TestNormalizeSupportedFieldTypes` covering string, number-not-string, date/time/datetime ranges, switch, single/multi choice label/value conversion, explicit optional `null`, required `null`, escaped plain `tui_editor`, exact `{"html":"..."}`, and empty attachments. Assert unknown fields and duplicate public name/alias use fail with stable field metadata.

- [ ] **Step 2: Write failing table and ambiguity tests**

Add `TestNormalizeTableColumnsToBackendKeys`, `TestNormalizeTableRejectsUnknownDuplicateAndMissingRequiredColumns`, `TestNormalizeRejectsAmbiguousChoiceLabel`, and `TestNormalizeDefaultPrecedence`. Assert explicit input > overlay default > live default > optional omission, display-only fields reject overrides, and duplicate names/choice labels never select the first match.

- [ ] **Step 3: Run field tests and confirm failure**

Run: `go test ./internal/servicedesk -run 'Test(DecodeTicketInput|Normalize)' -count=1`

Expected: FAIL because input normalization is undefined.

- [ ] **Step 4: Implement strict input normalization**

Implement:

```go
type TicketInput map[string]any
func DecodeTicketInput(data []byte) (TicketInput, error)
type NormalizedInput struct { Inputs map[string]any; Sources map[string]string; ProvidedFieldCount, DefaultedFieldCount, TableRowCount int }
func NormalizeInput(template Template, overlay *ResolvedOverlay, input TicketInput) (NormalizedInput, []FieldIssue)
```

Return all actionable field issues with public `field`, optional `column`, and zero-based `rowIndex`; do not expose backend keys in Agent errors unless URL mode has no public alternative.

- [ ] **Step 5: Write failing condition tests**

Add table tests for `==`, `!=`, `in`, `notin`, `>`, `<`, `>=`, `<=`, nested `and`/`or`, inactive required fields, active required fields, and an unknown expression that can affect visibility/requiredness. Assert counts for conditional, active, and unsupported conditions.

- [ ] **Step 6: Implement condition evaluation**

Implement:

```go
type ConditionResult struct { ConditionalFieldCount, ActiveConditionalFieldCount, UnsupportedConditionCount int; Active map[string]bool; Blocking []FieldIssue }
func EvaluateConditions(template Template, normalized NormalizedInput) ConditionResult
```

Evaluate against normalized values; an unsupported consequential condition blocks create rather than relying on a server-side 400.

- [ ] **Step 7: Write failing create dry-run tests**

Add `TestCreateDryRunPerformsNoWrite`, `TestCreateDryRunBuildsVerifiedRequestShape`, `TestCreateDryRunBlocksUnsupportedServiceKeyRequiredTypeConditionAndDestructiveDrift`, and `TestCreateDryRunStaleRegistryReportsBlockedApply`. Assert target/template/input/apply summaries, `contractVersion=1`, no full payload echo, and zero `start` calls.

- [ ] **Step 8: Implement create preflight and dry-run**

Implement:

```go
type CreateRequest struct { Target *EditTarget; Profile string; Input TicketInput; Apply bool }
type CreatePlan struct { Target ResolvedTarget; Template TemplateSummary; Input InputSummary; Apply ApplyPlan; /* private payload */ }
type CreateResult struct { Plan CreatePlan; Applied bool; Ticket *Ticket; Verify *CreateVerification; Outcome string }
type Dependencies struct { Client *Client; Registry RegistryLoader; Now func() time.Time }
func Create(ctx context.Context, deps Dependencies, request CreateRequest) (CreateResult, *DomainError)
```

Always load live metadata. Build the exact `start` payload in memory with `_loop: 0`, `retry: "true"`, and documented empty outer fields, but expose only safe summaries.

- [ ] **Step 9: Verify Task 5**

Run: `go test ./internal/servicedesk -run 'Test(DecodeTicketInput|Normalize|Condition|CreateDryRun)' -count=1`

Expected: PASS, including no-write assertions and Review Focus ambiguity cases.

- [ ] **Step 10: Commit Task 5**

```bash
git add internal/servicedesk/fields.go internal/servicedesk/fields_test.go internal/servicedesk/conditions.go internal/servicedesk/conditions_test.go internal/servicedesk/create.go internal/servicedesk/create_test.go
git commit -m "feat: plan validated ServiceDesk ticket creation"
```

### Task 6: Create Apply, Unknown Outcome, and Readback Verification

**Files:**
- Modify: `internal/servicedesk/create.go`
- Modify: `internal/servicedesk/create_test.go`
- Create: `internal/servicedesk/errors.go`
- Create: `internal/servicedesk/errors_test.go`

**Interfaces:**
- Consumes: `CreatePlan`, fixed-host `Client.Start/Get`, typed client errors, and current-time injection.
- Produces: apply behavior and stable `DomainError` codes used by every CLI operation.

- [ ] **Step 1: Write failing create apply tests**

Add `TestCreateApplyRepeatsPreflightThenWritesOnce`, `TestCreateApplyVerifiesIDTitleServiceAndReadableState`, `TestCreateApplyReadbackFailureRetainsTicketIDAndDetailURL`, `TestCreateApplyTimeoutReturnsUnknownWithoutRetry`, and `TestCreateApplyRegistryFreshnessRechecked`. The timeout test asserts one POST, `outcome=unknown`, safe title/request-start time/service context, no raw payload, and no automatic GET success claim.

- [ ] **Step 2: Write failing domain error/privacy tests**

Add `TestDomainErrorsHaveStableCodeRetryableAction`, `TestFieldErrorsCarryLocation`, and `TestAllSerializedErrorsRedactCredential`. Exercise auth, permission, Registry, protocol, drift, validation, state conflict, unknown outcome, malformed echo, and readback failure. Serialize to JSON and stderr-like text and assert the test credential is absent.

- [ ] **Step 3: Run apply/error tests and confirm failure**

Run: `go test ./internal/servicedesk -run 'Test(CreateApply|Domain|FieldErrors|AllSerialized)' -count=1`

Expected: FAIL because apply/error mapping is incomplete.

- [ ] **Step 4: Implement stable domain errors**

Implement:

```go
type DomainError struct { Code, Message, Action, Field, Column string; Retryable bool; RowIndex *int; Outcome string; Ticket *TicketReference }
func (e *DomainError) Error() string
func ClassifyError(operation string, err error) *DomainError
```

Keep message human-readable but require callers to branch on `Code`. Redact secrets before constructing both API and domain errors.

- [ ] **Step 5: Implement create apply and verification**

On `Apply=true`, rerun the complete preflight, enforce current Registry freshness, issue exactly one `Start`, and verify with `Get`. Once a ticket ID exists, retain its ID and safe detail URL in every later failure. Treat timeout/connection loss after request dispatch as unknown, never as confirmed failure.

- [ ] **Step 6: Verify Task 6**

Run: `go test ./internal/servicedesk -run 'Test(CreateApply|Domain|FieldErrors|AllSerialized)' -count=1`

Expected: PASS with one create POST under timeout and no credential leakage.

- [ ] **Step 7: Commit Task 6**

```bash
git add internal/servicedesk/create.go internal/servicedesk/create_test.go internal/servicedesk/errors.go internal/servicedesk/errors_test.go
git commit -m "feat: safely apply ServiceDesk ticket creation"
```

### Task 7: Ticket List, Get, and Withdraw

**Files:**
- Create: `internal/servicedesk/ticket.go`
- Create: `internal/servicedesk/ticket_test.go`

**Interfaces:**
- Consumes: `Client.UserInfo/List/Get/Withdraw`, current credential identity, URL parser, clock, and `DomainError`.
- Produces: `ListTickets`, `GetTicket`, `WithdrawTicket`, state categories, pagination metadata, and withdraw verification for CLI/skills.

- [ ] **Step 1: Write failing state/list tests**

Add `TestStateCategoryKnownAndUnknown`, `TestListMineRecentMergesScopesDeduplicatesAndSorts`, `TestListOpenIncludesActiveAndUnknownExcludesKnownTerminal`, `TestListLimitAppliesAfterDedupAndContinuesPaging`, and `TestListPartialScopeFailureIsIncomplete`. Assert `RUNNING`/`PAUSE` active, `WITHDRAWN`/`FINISHED`/`REVOKE` terminal, unknown states included in open, `instance_id` dedup, descending `created_time`, queried/failed scopes, cursor, `hasMore`, and `complete=false` on any partial failure.

- [ ] **Step 2: Run list tests and confirm failure**

Run: `go test ./internal/servicedesk -run 'Test(State|List)' -count=1`

Expected: FAIL because ticket operations are undefined.

- [ ] **Step 3: Implement get and multi-scope list**

Implement:

```go
type ListRequest struct { Mine bool; View string; Limit int; Cursor string }
type ListResult struct { Tickets []Ticket; Scopes []ScopeResult; Sort string; Cursor string; HasMore, Complete bool }
func ListTickets(ctx context.Context, client *Client, request ListRequest) (ListResult, *DomainError)
func GetTicket(ctx context.Context, client *Client, target DetailTarget) (Ticket, *DomainError)
func CategorizeState(state string) string
```

Query all userinfo business scopes, page until enough deduplicated results/all scopes exhausted/failure, and never describe partial results as complete. Keep cursor opaque and deterministic for resumption.

- [ ] **Step 4: Write failing withdraw tests**

Add `TestWithdrawDryRunReadsButDoesNotWrite`, `TestWithdrawRejectsOtherCreatorTerminalAndUnknownState`, `TestWithdrawReasonUnicodeLength`, `TestWithdrawApplyRechecksStateAndIdentity`, and `TestWithdrawApplyRequiresReadbackWithdrawn`. Assert trim then 1-500 Unicode code points, only own `RUNNING`/`PAUSE`, one `/withdraw/` call with the documented body, before/after states, and `verify.stateWithdrawn=true` only after readback.

- [ ] **Step 5: Implement withdraw preflight/apply**

Implement:

```go
type WithdrawRequest struct { Target DetailTarget; Reason string; Apply bool }
type WithdrawResult struct { DryRun, Applied bool; Ticket Ticket; BeforeState, AfterState string; Verify WithdrawVerification }
func WithdrawTicket(ctx context.Context, client *Client, request WithdrawRequest) (WithdrawResult, *DomainError)
```

Always read userinfo and ticket before planning; on apply repeat both checks, call only `/withdraw/`, and require readback state `WITHDRAWN`.

- [ ] **Step 6: Verify Task 7**

Run: `go test ./internal/servicedesk -run 'Test(State|List|Get|Withdraw)' -count=1`

Expected: PASS, including dedup/sort/partial-scope Review Focus coverage.

- [ ] **Step 7: Commit Task 7**

```bash
git add internal/servicedesk/ticket.go internal/servicedesk/ticket_test.go
git commit -m "feat: manage ServiceDesk ticket lifecycle"
```

### Task 8: CLI Command Surface and Doctor Integration

**Files:**
- Create: `cmd/ixf/servicedesk.go`
- Create: `cmd/ixf/servicedesk_test.go`
- Modify: `cmd/ixf/main.go`
- Modify: `cmd/ixf/main_test.go`
- Modify: `cmd/ixf/cli_integration_test.go`

**Interfaces:**
- Consumes: all `internal/servicedesk` operations and versioned result/error types.
- Produces: complete `ixf servicedesk` command tree, JSON/text rendering, exit codes, and `services.servicedesk` root-doctor summary.

- [ ] **Step 1: Write failing CLI help/flag tests**

Cover help for `doctor`, `template inspect`, `profile discover|validate|list|show`, `create`, `list`, `get`, and `withdraw`. Assert exact mutually exclusive pairs (`--url/--profile`, `--dry-run/--apply`, detail URL/ticket ID), required business IDs, `--mine` only, valid views, positive limits, input/output file requirements, `profile discover` never publishes to Registry, no `--api-key` flag, and usage errors exit 2.

- [ ] **Step 2: Write failing JSON/exit-contract tests**

Using injectable package-level adapters consistent with existing CLI tests, assert success and domain-failure output is exactly one JSON document with `operation`, `contractVersion=1`, `ok`, stable error fields, and exit 1 for domain failures. Assert text mode remains human-readable but Agents can rely only on JSON.

- [ ] **Step 3: Implement the CLI dispatcher**

Add `case "servicedesk"` in root dispatch and keep all new parsing/rendering in `servicedesk.go`:

```go
func runServiceDesk(args []string, stdout, stderr io.Writer) int
```

Map commands to package requests without rebuilding payload logic. Default create/withdraw to dry-run when `--apply` is absent; reject explicit `--dry-run --apply`.

- [ ] **Step 4: Write failing standalone/root doctor tests**

Assert standalone doctor reports credential source/configuration, optional business ID, online userinfo result, and skipped probe reason without secret/length. Assert root doctor adds `services.servicedesk`, does not change overall `ok` when unconfigured or probe-skipped, and distinguishes authentication/permission/network status when configured.

- [ ] **Step 5: Implement doctor integration**

Add read-only standalone diagnostics and inject a ServiceDesk summary into `collectDiagnostics` output. Do not mutate files, fetch Registry, or require ServiceDesk for existing Toolbox capability readiness.

- [ ] **Step 6: Add CLI fixture integration tests**

Run the built command against transport-injected fixture clients for template inspect, create dry-run/apply, profile list/show/validate, list/get, and withdraw dry-run/apply. Assert no test reaches the network and no committed environment contains a real key.

- [ ] **Step 7: Verify Task 8**

Run: `go test ./cmd/ixf ./internal/servicedesk -count=1`

Expected: PASS for command coverage, output contracts, doctor isolation, and package tests.

- [ ] **Step 8: Commit Task 8**

```bash
git add cmd/ixf/servicedesk.go cmd/ixf/servicedesk_test.go cmd/ixf/main.go cmd/ixf/main_test.go cmd/ixf/cli_integration_test.go
git commit -m "feat: expose Agentic ServiceDesk CLI"
```

### Task 9: Agent Skills, Claude Routing Hook, and Documentation

**Files:**
- Create: `skills/ixf-servicedesk-reader/SKILL.md`
- Create: `skills/ixf-servicedesk-writer/SKILL.md`
- Modify: `skills/using-ixf-toolbox/SKILL.md`
- Modify: `plugin-src/claude/hooks/user-prompt-submit`
- Modify: `internal/pluginpack/pluginpack.go`
- Modify: `internal/pluginpack/pluginpack_test.go`
- Modify: `internal/pluginpack/bootstrap_test.go`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `docs/agent-routing.md`
- Modify: `docs/go-python-parity.md`

**Interfaces:**
- Consumes: the stable CLI JSON commands and safety sequence from Task 8.
- Produces: native Codex/Claude discovery, reader/writer operating instructions, routing tests, and current public documentation.

- [ ] **Step 1: Read the writing-skills guidance before editing canonical skills**

Run: `sed -n '1,260p' /Users/chenqian/.codex/skills/writing-skills/SKILL.md`

Expected: the implementer records its validation requirements and follows them for both new skills.

- [ ] **Step 2: Write failing plugin/routing tests**

Extend canonical skill expectations with `ixf-servicedesk-reader` and `ixf-servicedesk-writer`. Add hook fixtures for the ServiceDesk host and explicit intents: supported ticket types/list/status -> reader; create/report repair/withdraw -> routing skill then writer. Add negative fixtures such as “服务器硬盘坏了，帮我分析原因” and local Markdown requests. Assert the hook adds routing context without embedding service/profile IDs or claiming an automatic keyword-selected profile.

- [ ] **Step 3: Add canonical ServiceDesk skills and route rules**

Reader skill covers profile list/show, template inspect/validate, and ticket list/get. Writer skill enforces semantic profile selection, current-context fact reuse, missing-fact questions, inspect -> input -> dry-run -> explicit confirmation -> apply -> readback; withdrawal uses list-based semantic narrowing, a fresh confirmation, and never revoke/terminate. Both skills resolve the packaged Go runtime using the existing plugin convention and never ask users to paste API Keys into chat.

- [ ] **Step 4: Update Claude prompt hook**

Add `servicedesk.iflytek.com` and explicit ticket capability/action intent to hook routing while preserving bypass phrases and ordinary-fault negative behavior. Hook output must still direct Claude first to `ixf-toolbox:using-ixf-toolbox`; the canonical routing skill selects reader/writer.

- [ ] **Step 5: Update current docs**

Document natural-language entry, intent gate, Agent semantic profile routing, live field mapping, API Key sources, Registry/cache, CLI JSON contract, four v1 user scenarios, unsupported workflows/attachments, and the CLI/plugin/skill boundary in both READMEs and routing/parity docs. Keep commands as an integration reference, not the primary user experience.

- [ ] **Step 6: Generate and verify plugin packages**

Run: `go run ./cmd/pluginpack`

Then run: `go run ./cmd/pluginpack --check && go test ./internal/pluginpack -count=1`

Expected: PASS; both generated plugin roots contain the new canonical skills and ServiceDesk hook behavior.

- [ ] **Step 7: Commit Task 9**

```bash
git add skills plugin-src/claude/hooks internal/pluginpack README.md README.en.md docs/agent-routing.md docs/go-python-parity.md plugins .agents .claude-plugin
git commit -m "feat: route Agents to ServiceDesk workflows"
```

### Task 10: Version, Registry Publication, Live Acceptance, and Release Readiness

**Files:**
- Modify: `VERSION`
- Modify: generated plugin manifests/runtime metadata under `.agents/`, `.claude-plugin/`, and `plugins/`
- Create outside repository: `/disk1/files/ixf-toolbox/servicedesk/v1/index.json`
- Create outside repository: `/disk1/files/ixf-toolbox/servicedesk/v1/profiles/server-hardware-repair-test.json`

**Interfaces:**
- Consumes: completed CLI, discover/validate, pluginpack, approved test target `businessId=15/serviceId=4003`, user-provided API Key supplied only through an ephemeral environment/file source, and Registry maintenance path.
- Produces: version `3.32.0`, first validated shared overlay, end-to-end acceptance evidence, and a branch ready for independent review/PR.

- [ ] **Step 1: Bump the feature version and regenerate packages**

Change `VERSION` from `3.31.3` to `3.32.0`, run `go run ./cmd/pluginpack`, and assert generated `runtime.json`/manifests use `3.32.0`.

- [ ] **Step 2: Run repository verification before any live write**

Run:

```bash
gofmt -w cmd/ixf/servicedesk.go cmd/ixf/servicedesk_test.go internal/servicedesk/*.go
go run ./cmd/pluginpack --check
go test ./...
go vet ./...
git diff --check
```

Expected: all commands exit 0 on the supported local platform; CI later confirms macOS, Linux, and Windows.

- [ ] **Step 3: Discover and validate the first overlay without persisting credentials**

Provide the user's key through an ephemeral `IXF_SERVICEDESK_API_KEY` process environment or a temporary `0600` file outside the repository. Run `profile discover` for the approved edit URL, fill only the approved routing display/use/do-not-use/examples, then run `profile validate --json`. Do not add unapproved defaults or required strengthening.

- [ ] **Step 4: Publish Registry files atomically**

Write the validated overlay to the maintenance directory, compute SHA-256 from exact bytes, create strict `index.json`, stage both under a temporary sibling directory, then atomically rename into `v1`. Verify `profile list/show --json` downloads from `http://10.103.251.43/files/ixf-toolbox/servicedesk/v1/` and reports `source=network`, `stale=false`.

- [ ] **Step 5: Run read-only live acceptance**

Run standalone doctor, profile list/show, template inspect, ticket recent/open list, and a create dry-run using clearly marked test content. Confirm no credential appears in stdout/stderr and present the dry-run target/content summary to the user.

- [ ] **Step 6: Obtain explicit user confirmation for the live create**

Do not infer approval from design or plan approval. Show service display name, title, field/table-row summary, defaults, drift, and `apply.blocked`; wait for confirmation specific to this test ticket.

- [ ] **Step 7: Apply, query, and prepare withdrawal**

Create once with `--apply`, verify the returned ticket is readable and `RUNNING`, confirm it appears in recent/open lists, and use list output plus business description to select the unique test ticket. Run withdraw dry-run with a test reason and show ID/name/title/state/reason.

- [ ] **Step 8: Obtain explicit user confirmation for withdrawal and apply**

Wait for a new withdrawal-specific confirmation, then apply. Verify readback is `WITHDRAWN` and the ticket no longer appears in the open view while remaining in recent.

- [ ] **Step 9: Commit release-ready generated/version changes**

```bash
git add VERSION .agents .claude-plugin plugins
git commit -m "chore: prepare v3.32.0"
```

- [ ] **Step 10: Request independent code review**

Use the `requesting-code-review` skill for a whole-branch review against the approved spec and this plan. Resolve findings with focused tests and commits; rerun the full verification commands.

- [ ] **Step 11: Prepare the PR without tagging or releasing**

Confirm the branch contains no credentials or live private payloads, push through the required `127.0.0.1:7890` GitHub proxy, and open a PR to `main` summarizing protocol coverage, safety boundaries, Registry publication, live acceptance, and test evidence. Tag/release only after PR checks pass and the user approves merge/release.
