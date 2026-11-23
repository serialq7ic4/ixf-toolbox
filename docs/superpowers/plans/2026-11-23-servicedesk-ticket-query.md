# ServiceDesk Ticket Query Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ambiguous recent/open ticket list with explicit todo, submitted-open, and contextual candidate-search workflows that Agents can use without cookies or UI automation.

**Architecture:** Keep ServiceDesk HTTP and pagination deterministic in the Go runtime, move query aggregation into a focused `internal/servicedesk/query.go`, and leave semantic candidate selection to the Agent. The CLI exposes role/state/search contracts; canonical skills translate natural-language intent into those atomic commands and call `get` after selecting a candidate.

**Tech Stack:** Go 1.24+, standard `net/http`, `encoding/json`, `flag`, table-driven Go tests, canonical Markdown skills, pluginpack generation.

**Spec:** `docs/superpowers/specs/2026-11-23-servicedesk-ticket-query-design.md`

## Global Constraints

- Go-only and API-first: no Python fallback, browser Cookie, HAR replay, UI automation, or arbitrary HTTP endpoint.
- API Key values never enter command arguments, JSON output, fixtures, logs, or Agent prompts.
- `list`, `search`, and `get` remain read-only; create and withdraw confirmation/apply behavior must not change.
- ServiceDesk `q` was verified on 2026-11-23 to match display ticket number and full title but not service name, so v1 search must fully read scopes and filter locally.
- Query filtering uses Unicode lowercase plus Unicode-whitespace tokenization; every token must be a substring of concatenated `name`, `title`, and `serviceName`. No fuzzy matching, synonym table, Chinese segmentation, pinyin, edit distance, or Unicode normalization.
- Aggregate output is globally limited only after full pagination, filtering, and deduplication; v1 has no cursor.
- Keep the unreleased feature version at `3.32.0`; this work completes that minor release rather than opening `3.33.0`.
- Every behavior change follows RED-GREEN TDD and each task gets its own commit.

## Review Focus

- Duplicate records returned through aggregate/dedicated businesses and apply/wait must merge roles and choose fields by the documented deterministic precedence; Task 3 pins every tie-breaker.
- A partial scope failure must return partial tickets with `complete=false` and `hasMore=null`, while total failure before any usable data must return `query_failed`; Task 3 covers both.
- Submitted-open must retain unknown states while excluding only known terminal states; Task 2 covers this.
- Local search must handle Unicode case folding, repeated whitespace, AND-token semantics, and no accidental service-name omission; Task 3 covers this.
- Ambient bootstrap business ID must never replace a candidate's actual business ID, and explicit flags must win over environment/file defaults; Task 4 covers this.

---

### Task 1: Parameterize the ServiceDesk List Transport

**Files:**
- Modify: `internal/servicedesk/client.go`
- Modify: `internal/servicedesk/client_test.go`

**Interfaces:**
- Consumes: ServiceDesk `GET /api/taskflow3/v3/taskflow/` with `key=request`.
- Produces: `ListQuery{FlowType string, Limit int, Offset int}` and `(*Client).List(context.Context, int, ListQuery) (TicketPage, error)` supporting exactly `apply` and `wait`.

- [ ] **Step 1: Write failing transport tests**

Add `TestClientListUsesRequestedFlowType` with table cases for `apply` and `wait`. Each case must assert the exact sorted query parameters include `flow_type=<value>`, `key=request`, `limit=20`, and `offset=40`.

Add `TestClientListRejectsUnsupportedFlowTypeBeforeTransport` with an atomic transport-call counter. Assert empty, `any`, and unknown values return `APIError.Code == "invalid_request"` without calling HTTP.

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `go test ./internal/servicedesk -run 'TestClientList(UsesRequestedFlowType|RejectsUnsupportedFlowTypeBeforeTransport)' -count=1`

Expected: FAIL because `ListQuery` has no `FlowType` and the client hardcodes `apply`.

- [ ] **Step 3: Implement explicit flow selection**

Extend `ListQuery` with `FlowType string`. Update `Client.List` to validate exactly `apply|wait` before request creation and use the validated value in `flow_type`. Update existing direct `Client.List` tests to pass `FlowType: "apply"`; do not add a default that could silently choose the wrong role.

- [ ] **Step 4: Run transport and ServiceDesk package tests**

Run: `go test ./internal/servicedesk -run 'TestClient' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/servicedesk/client.go internal/servicedesk/client_test.go
git commit -m "feat: support ServiceDesk ticket flow roles"
```

### Task 2: Build Role-Aware Full-Scope Aggregation

**Files:**
- Create: `internal/servicedesk/query.go`
- Create: `internal/servicedesk/query_test.go`
- Modify: `internal/servicedesk/ticket.go`
- Modify: `internal/servicedesk/ticket_test.go`

**Interfaces:**
- Consumes: Task 1 `Client.List` with explicit `FlowType` and existing `Client.UserInfo`.
- Produces:
  - `type QueryRole string` with `QueryRoleSubmitted`, `QueryRoleTodo`, and `QueryRoleAny`.
  - `type QueryStateFilter string` with `QueryStateAll` and `QueryStateOpen`.
  - `type QueryRequest struct { BootstrapBusinessID int; Mine bool; Role QueryRole; State QueryStateFilter; Query string; Limit int }`.
  - `type ScopeResult struct { BusinessID int; Name string; FlowType string; TicketCount int; Complete bool; Error string }`.
  - `type FailedScope struct { BusinessID int; FlowType string; Error string }`.
  - `type QueryResult struct { ContractVersion int; Role QueryRole; StateFilter QueryStateFilter; Query string; BootstrapBusinessID int; Tickets []Ticket; Scopes []ScopeResult; FailedScopes []FailedScope; Sort string; HasMore *bool; Complete bool }`, with `HasMore` tagged `json:"hasMore"` and no `omitempty` so incomplete results encode as `null`.
  - `func QueryTickets(context.Context, *Client, QueryRequest) (QueryResult, *DomainError)`.
  - `Ticket.RoleMatches []string` with `json:"roleMatches,omitempty"`.
- Removes: `ListRequest`, `ListResult`, and `ListTickets`; `ticket.go` retains ticket normalization, state categorization, get, and withdraw behavior.

- [ ] **Step 1: Write failing request-validation and role-mapping tests**

In `query_test.go`, add `TestQueryTicketsValidatesRoleStateAndMine` asserting invalid bootstrap ID, `Mine=false`, empty/unknown role, and `role=todo,state=open` return `invalid_request` before HTTP.

Add `TestQueryTicketsMapsSubmittedAndTodoFlows` with a fixture transport that records requests. Assert submitted uses only `apply`, todo uses only `wait`, and todo results are not filtered by creator.

- [ ] **Step 2: Write failing pagination, state, and result-contract tests**

Add `TestQueryTicketsFullyPaginatesScopesBeforeGlobalLimit`: two businesses, multiple pages, cross-scope duplicates, global limit 2. Assert all pages were requested, output is sorted by `updateTime` then `createdTime`, `complete=true`, `hasMore` points to `true`, and scopes include flow type and raw counts.

Add `TestQueryTicketsSubmittedOpenExcludesTerminalAndKeepsUnknown`: `FINISHED`, `WITHDRAWN`, and `REVOKE` are absent; `RUNNING`, `PAUSE`, and an unknown state remain.

Add `TestQueryTicketsZeroBusinessScopesIsCompleteEmpty`: userinfo with no businesses returns `ok` domain result, empty tickets/scopes, `complete=true`, and `hasMore=false`.

Add `TestQueryTicketsBoundsScopeConcurrency`: block six scope requests, assert observed concurrent requests are greater than one and never exceed the constant `maxConcurrentServiceDeskScopes = 4`, then release and require deterministic scope order.

- [ ] **Step 3: Run query tests and verify RED**

Run: `go test ./internal/servicedesk -run 'TestQueryTickets' -count=1`

Expected: FAIL because `query.go` and the role-aware contracts do not exist.

- [ ] **Step 4: Implement single-role aggregation**

Create `query.go` with the exact public types above. Implement submitted and todo validation, userinfo expansion, four-worker bounded scope concurrency, sequential pagination within each scope, known-state filtering, composite `(response business_id, instance_id)` deduplication, deterministic scope ordering, global limit, and nullable `HasMore`.

Move list-only types and behavior out of `ticket.go`; leave `CategorizeState`, `normalizeTicket`, get, and withdraw unchanged. Initially reject `QueryRoleAny` and non-empty `Query` with `invalid_request`; Task 3 adds them.

- [ ] **Step 5: Run focused and package tests**

Run: `go test ./internal/servicedesk -run 'Test(QueryTickets|StateCategory|Withdraw)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/servicedesk/query.go internal/servicedesk/query_test.go internal/servicedesk/ticket.go internal/servicedesk/ticket_test.go
git commit -m "feat: aggregate ServiceDesk ticket roles"
```

### Task 3: Add Contextual Search, Any-Role Merge, and Partial Results

**Files:**
- Modify: `internal/servicedesk/query.go`
- Modify: `internal/servicedesk/query_test.go`
- Modify: `internal/servicedesk/errors.go`
- Modify: `internal/servicedesk/errors_test.go`

**Interfaces:**
- Consumes: Task 2 `QueryTickets`, role/state types, scope outcomes, and `Ticket.RoleMatches`.
- Produces:
  - `QueryRoleAny` executes both `apply` and `wait` for every business.
  - `func matchesTicketQuery(Ticket, string) bool` implementing the approved local AND-token rule.
  - `query_failed` as a stable DomainError code/action when no scope completes and no ticket is recovered.

- [ ] **Step 1: Write failing local-search tests**

Add `TestMatchesTicketQueryUsesUnicodeCasefoldWhitespaceAndAllTokens`. Assert tokens can match across `name`, `title`, and `serviceName`; repeated Unicode whitespace is ignored; ASCII case differences match; one absent token rejects; empty query matches all only for list requests and is rejected by search at the CLI boundary.

Add `TestQueryTicketsSearchFiltersAfterFullPagination`. Provide a match that appears only on a later page and only in `serviceName`; assert it is found without a `q` URL parameter and unrelated tickets are absent.

- [ ] **Step 2: Write failing any-role and duplicate-precedence tests**

Add `TestQueryTicketsAnyMergesRolesAndDuplicatePrecedence` with duplicate tickets spanning aggregate/dedicated business scopes and apply/wait. Assert `roleMatches` is the stable ordered union `["submitted","todo"]`; newer `updateTime` wins, then dedicated scope, then wait, then lower request-scope business ID. Invalid times rank oldest.

Add `TestQueryTicketsAnyOpenFiltersOnlySubmitted`: terminal submitted records are excluded, while todo records remain regardless of terminal-looking state; shared tickets retain todo role.

- [ ] **Step 3: Write failing partial/failure tests**

Add `TestQueryTicketsPartialResultHasUnknownMore`: one scope succeeds or yields a page before failing and another fails. Assert `complete=false`, `hasMore == nil`, flow-specific `FailedScopes`, retained partial tickets, and stable scope errors.

Add `TestQueryTicketsReturnsQueryFailedWithoutUsableScope`: every scope fails before a page yields data. Assert `DomainError.Code == "query_failed"` and no successful result envelope is implied.

- [ ] **Step 4: Run focused tests and verify RED**

Run: `go test ./internal/servicedesk -run 'Test(MatchesTicketQuery|QueryTickets)' -count=1`

Expected: FAIL on any-role, search, duplicate precedence, nullable partial state, and `query_failed` behavior.

- [ ] **Step 5: Implement local filtering and deterministic merge**

Implement the exact text rule with `strings.ToLower` and `strings.Fields`; concatenate `Name`, `Title`, and `ServiceName`. Extend scope generation for any-role, retain source metadata through merging, apply the documented duplicate precedence, union roles, and sort by update time, created time, business ID, then instance ID.

Return partial data whenever any scope completes or any ticket was recovered; set `HasMore=nil`. Return `query_failed` only when no scope completes and no ticket exists. Add the stable action text to `actionForCode`.

- [ ] **Step 6: Run package and race tests**

Run: `go test ./internal/servicedesk -count=1`

Expected: PASS.

Run: `go test -race ./internal/servicedesk -count=1`

Expected: PASS with no race report.

- [ ] **Step 7: Commit**

```bash
git add internal/servicedesk/query.go internal/servicedesk/query_test.go internal/servicedesk/errors.go internal/servicedesk/errors_test.go
git commit -m "feat: search ServiceDesk ticket candidates"
```

### Task 4: Expose Role, State, and Search CLI Contracts

**Files:**
- Create: `cmd/ixf/servicedesk_query.go`
- Create: `cmd/ixf/servicedesk_query_test.go`
- Modify: `cmd/ixf/servicedesk.go`
- Modify: `cmd/ixf/servicedesk_test.go`

**Interfaces:**
- Consumes: Task 3 `QueryTickets`, `QueryRequest`, roles, state filters, and `BusinessIDLoader.Load(int)`.
- Produces:
  - `ixf servicedesk list --mine --role todo|submitted [--state all|open] [--business-id ID] [--limit N] --json`.
  - `ixf servicedesk search --mine [--role any|todo|submitted] [--state all|open] --query TEXT [--business-id ID] [--limit N] --json`.
  - Operations `servicedesk_list` and `servicedesk_search` using the existing envelope.
  - `var serviceDeskQueryRunner = ixfsd.QueryTickets` for request-capture tests.
- Removes: `--view` and `--cursor` from the unreleased list command.

- [ ] **Step 1: Write failing command-tree and parser tests**

Add `search` to `TestServiceDeskHelpListsCommandTree`.

In `servicedesk_query_test.go`, add `TestServiceDeskListAndSearchBuildQueryRequests` using `serviceDeskQueryRunner` to capture requests. Cover list submitted-open, list todo with no explicit state, search defaults to any/all, search any/open, and explicit limit/query trimming.

Add `TestServiceDeskQueryRejectsInvalidContractsBeforeCredentialLookup`: missing `--mine`, list missing/any role, todo with explicitly supplied state, empty search query, nonpositive limit, `--view`, and `--cursor` all exit 2 without calling the credential loader.

- [ ] **Step 2: Write failing bootstrap business ID tests**

Add `TestServiceDeskQueryBusinessIDPrecedence`: explicit `--business-id` wins over `IXF_SERVICEDESK_BUSINESS_ID`; absent flag uses the environment; absent flag/environment uses a 0600 default file under a temporary HOME. Assert the captured request carries only the bootstrap ID and that returned candidate business IDs are untouched.

Add a JSON failure case for no configured bootstrap ID. Assert operation attribution remains list/search and the stable code is `invalid_request` without printing credential paths or secret values.

- [ ] **Step 3: Run CLI tests and verify RED**

Run: `go test ./cmd/ixf -run 'TestServiceDesk(Query|ListAndSearch|HelpLists)' -count=1`

Expected: FAIL because search and the new flags do not exist.

- [ ] **Step 4: Implement focused CLI handlers**

Move list parsing out of `servicedesk.go` into `servicedesk_query.go`, add search dispatch/help, detect whether `--state` was explicitly set with `FlagSet.Visit`, and resolve the bootstrap ID through `BusinessIDLoader`. A missing bootstrap value must produce a JSON `invalid_request` envelope when `--json` is requested; syntax/unsupported-flag errors remain exit 2.

Call `serviceDeskClientLoader` only after local contract and bootstrap validation. Pass the exact request to `serviceDeskQueryRunner`, and preserve result completeness/nullable `hasMore` without post-processing in the CLI.

- [ ] **Step 5: Run CLI and ServiceDesk tests**

Run: `go test ./cmd/ixf ./internal/servicedesk -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/ixf/servicedesk.go cmd/ixf/servicedesk_test.go cmd/ixf/servicedesk_query.go cmd/ixf/servicedesk_query_test.go
git commit -m "feat: expose ServiceDesk ticket query modes"
```

### Task 5: Teach Agents the Three Query Workflows

**Files:**
- Modify: `skills/ixf-servicedesk-reader/SKILL.md`
- Modify: `skills/ixf-servicedesk-writer/SKILL.md`
- Modify: `skills/using-ixf-toolbox/SKILL.md`
- Modify: `docs/agent-routing.md`
- Modify: `README.md`
- Modify: `README.en.md`
- Modify: `CHANGELOG.md`
- Modify: `repository_contract_test.go`
- Generate: `plugins/codex/ixf-toolbox/skills/ixf-servicedesk-reader/SKILL.md`
- Generate: `plugins/codex/ixf-toolbox/skills/ixf-servicedesk-writer/SKILL.md`
- Generate: `plugins/codex/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md`
- Generate: `plugins/claude/ixf-toolbox/skills/ixf-servicedesk-reader/SKILL.md`
- Generate: `plugins/claude/ixf-toolbox/skills/ixf-servicedesk-writer/SKILL.md`
- Generate: `plugins/claude/ixf-toolbox/skills/using-ixf-toolbox/SKILL.md`

**Interfaces:**
- Consumes: Task 4 CLI commands and the approved Agent selection rules.
- Produces: canonical routing guidance for todo, submitted-open, contextual search-then-get, and withdraw confirmation isolation; generated host plugin copies exactly match canonical skills.

- [ ] **Step 1: Write failing repository contract assertions**

Extend `TestServiceDeskSkillsAreCanonicalAndGenerated` or add `TestServiceDeskReaderDocumentsAtomicQueryModes`. Require canonical reader guidance to contain `--role todo`, `--role submitted --state open`, `search --mine --role any`, automatic read-only candidate selection followed by `get`, `complete:false`, and no browser Cookie guidance. Require writer guidance to retain fresh withdraw confirmation and use submitted-open/search only to identify a candidate.

- [ ] **Step 2: Run the contract test and verify RED**

Run: `go test . -run 'TestServiceDesk(ReaderDocumentsAtomicQueryModes|SkillsAreCanonicalAndGenerated)' -count=1`

Expected: FAIL because canonical skills still document `--view recent|open`.

- [ ] **Step 3: Update canonical skills and routing docs**

Teach the reader these exact workflows:

- todo intent -> `list --role todo`;
- submitted unfinished intent -> `list --role submitted --state open`;
- contextual single-ticket intent -> `search --role any`, Agent selects the most semantically relevant and most recently updated credible candidate, then `get` with the candidate's actual business/internal ID;
- `REQ...` -> exact search, while detail URL or explicit business/internal ID -> direct get;
- partial results are disclosed and are never called complete.

Keep the writer stricter: search can locate withdrawal candidates, but ambiguous targets require user choice and every withdraw still needs dry-run plus fresh confirmation. Update the routing skill and `docs/agent-routing.md` without embedding CLI keyword scoring.

- [ ] **Step 4: Update user-facing command references and changelog**

Replace recent/open commands in both READMEs with todo, submitted-open, and search examples. Update the existing unreleased `3.32.0` changelog entry rather than changing `VERSION`.

- [ ] **Step 5: Generate plugin copies and verify them**

Run: `go run ./cmd/pluginpack`

Expected: canonical ServiceDesk skill edits are copied into both Codex and Claude plugin trees.

Run: `go run ./cmd/pluginpack --check`

Expected: PASS with no output.

- [ ] **Step 6: Run contract and full tests**

Run: `go test . -count=1`

Expected: PASS.

Run: `go test ./... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add skills/ plugins/ docs/agent-routing.md README.md README.en.md CHANGELOG.md repository_contract_test.go
git commit -m "docs: route ServiceDesk ticket query intents"
```

## Final Verification

Run after all task commits and before branch integration:

```bash
gofmt -w cmd/ixf/servicedesk*.go internal/servicedesk/*.go
go test ./... -count=1
go test -race ./internal/servicedesk ./cmd/ixf -count=1
go vet ./...
go run ./cmd/pluginpack --check
git diff --check
go build -o /tmp/ixf-toolbox-3.32.0-bin/ixf ./cmd/ixf
```

Perform API Key-only, read-only live acceptance with the rebuilt binary:

1. `/tmp/ixf-toolbox-3.32.0-bin/ixf servicedesk list --mine --role todo --business-id 9 --limit 20 --json` returns flow-specific complete scopes or explicitly reports partial scope failures.
2. `/tmp/ixf-toolbox-3.32.0-bin/ixf servicedesk list --mine --role submitted --state open --business-id 9 --limit 20 --json` returns active/unknown submitted tickets, with no known terminal state.
3. Derive a private query from one returned ticket without printing it, run `search --mine --role any`, and verify the selected candidate can be passed to `get` using its actual business/internal ID.
4. Summarize only counts, states, completeness, and boolean match checks; do not print ticket titles, people, API Keys, or raw responses.
5. Delete all temporary response files. Do not create or withdraw a ticket.
