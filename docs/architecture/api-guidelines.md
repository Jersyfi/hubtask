# API Guidelines (API First)

The contract is [`api/openapi.yaml`](../../api/openapi.yaml) (OpenAPI 3.1), written **before** the
code; server interfaces and client SDKs are generated from it (`oapi-codegen`), and CI fails when
they drift apart ([ADR-0004](../adr/ADR-0004-api-first-openapi.md)).

---

## 1. Ground rules

1. **Nothing exists only in the UI.** Every use case is reachable through the API (coverage test against `usecase.Registry`).
   **The doors are generated** (F-7): an operation marked `x-hubtask-rest: generated` is answered by
   the mapping `tools/doors` writes into `presentation/rest/Doors.gen.go` - the request body
   decoded into its generated type first, its members, the path's and the query's parameters
   handed to the use case by the contract's names (the operationId is the use case's), `If-Match`
   as `expected_version` (or as the text `x-hubtask-rest-if-match` names), `X-Hubtask-Step-Up` as
   `step_up_token`, the output passed through the generated answer type, its integer `version`
   (or the member `x-hubtask-rest-etag` names) as the ETag; `x-hubtask-rest-path` renames a path
   parameter's input, `x-hubtask-rest-body` hands the whole body on as one input (a form),
   `x-hubtask-rest-location` fills a created resource's `Location`; an explicit `null` member
   reaches the use case as present and null, as MCP's arguments carry it, or as the empty text
   where `x-hubtask-rest-null: empty` says the use case reads that as *clear*. A path parameter
   renamed to `""` only places the resource. `x-hubtask-rest-query` renames a query parameter's
   input (`size` read as `limit`); `x-hubtask-rest-use-case` sends a door to another use case than
   its operationId (creating and replacing an identity provider are one use case, told apart by an
   identifier); `x-hubtask-rest-shown-once` names the member that shows a secret once - an answer
   carrying it is marked `Cache-Control: no-store` and `X-Hubtask-Shown-Once` for the idempotency
   record (T-18); `x-hubtask-rest-list: data` draws a list under `data` without a page row by row.
   The use case's input therefore uses
   the contract's names; a test holds each mapping's query, version and proof to the descriptor's
   declared input. An answer drawn by one of the adapter's shared mappers (an entry, by
   `workItemResponse`) names it with `x-hubtask-rest-answer`; for a bare-array or paged answer
   the named mapper draws one row, so a mapper that draws the whole list keeps its handler. `x-hubtask-cli` names
   the `hubctl` command the same tool writes into `cmd/hubctl/Doors.gen.go`; a hand-written
   command of the same name is kept.
2. **One major path:** `/api/v1`. Additive changes (new fields, new endpoints, new enum values in *responses*) are compatible and need no new version.
3. **Clients must be tolerant:** ignore unknown fields. This is documented in the contract.
4. **Self-describing:** `GET /api/v1/meta/capabilities` answers the vocabulary (item types, capability profiles, field types, enum values, limits, locales, query fields, automation, event types); frontends and agents configure themselves from it rather than hard-coding. An optional surface is announced under `features`; a client that does not find its flag renders what it had before and calls no route of that surface.
5. **No display text from the server** (see [i18n-l10n.md](./i18n-l10n.md)) — codes and parameters only.
6. **Consistent plural resource names**, `snake_case` for JSON fields, ISO-8601/RFC 3339 for instants, ISO-8601 durations for relative values, IANA names for time zones.

---

## 2. Resource overview

[`api/openapi.yaml`](../../api/openapi.yaml) lists every route and operation; paths are below
`/api/v1`. What the shape of a route decides:

* **Actions** use the suffix pattern `POST /items/{id}:complete` (Google AIP style) — clearer than
  status fields for operations with side effects, and easier for agents.
* `/tenant` is one workspace as its members see it (display name, the members' two defaults, the
  administrators' second-factor switch); `/admin/…` crosses workspaces and is the installation
  operator's.
* **Search is `POST /search` only**: what somebody looks for is their content, and a query string
  travels through access logs, proxies and browser history. `/items:query` is a `POST` for the same
  reason.
* A collection's buckets and labels are answered as a plain array, unpaged.
* An entry's labels, members and attachments are `PUT`/`DELETE` on
  `/items/{id}/{labels|members|attachments}/{id}`. A custom field value is `PUT` one key per call at
  `/items/{id}/custom-fields/{key}` (`null` clears), because the merge rule is per key. The due trio
  travels together at `/items/{id}/due`, and the same fields on create and update dispatch into the
  same writer. A reminder or recurrence a client can set it can also read back.
* `/items/{id}/activity` is the only reader of an entry's history.
* Deleting a backup target a schedule still names is a `409`; nothing at the target is ever touched.
* A data subject request's deadline moves only through `POST /privacy/requests/{id}:extend`, once;
  `PATCH /privacy/requests/{id}` writes no date. Its two days are calendar days (`format: date`) in
  the workspace's time zone, and a case answers `extendable_until` exactly while an extension can
  succeed ([data-protection.md](./data-protection.md) §4.1).
* What legal holds keep of an erasure is read, never written, by a client: `GET
  /privacy/requests/{id}/erasure-preview` before it starts, and a case's `kept` and
  `kept_legal_basis` after - present only where a hold kept something. A partly completed case is
  `COMPLETED` ([data-protection.md](./data-protection.md) §4.1).
* The intake doors (`/jumble/inbound/{token}`, `/jumble/mail/{token}`, `/automation/inbound/{token}`)
  authenticate a tenant or a rule, never a person, are capped, and answer every reason not to serve
  with the same `404`.
* The configuration document is `/config` (its export, the hash as `ETag`) with the actions
  `:validate`, `:plan`, `:propose` and `:apply`; an apply names the plan's `plan_hash` and is refused
  `409 config.version_conflict` when the configuration moved since, through any door; a plan that
  says `step_up` (it changes the sign-in rule) is applied with `X-Hubtask-Step-Up`. Every refusal
  of a plan is a message code with the key path (`types.incident.level`), never prose. The same use
  cases are MCP tools by the registry's names (`plan_config`, `apply_config`, …) and `hubctl config`.
  A hub's own document is the same doors with `hub_id` (a query parameter on the reads, a body
  field on the actions); the sandbox is `/config/sandbox` (`GET` with its promotion's plan, `POST`
  to change, `reset` or `discard` it), promoted by `:plan` and `:apply` with `from_sandbox`.
* `/mcp` (streamable HTTP, [ai-first.md](./ai-first.md)) and `/caldav/` (WebDAV, §7) are outside the
  OpenAPI document.

---

## 3. The query DSL (the basis of every view)

One endpoint serves list, board, and timeline: `POST /api/v1/items:query`.

```json
{
  "scope": { "container_id": "018f...", "include_descendants": true },
  "filter": {
    "op": "AND",
    "nodes": [
      { "field": "type", "op": "IN", "value": ["TASK"] },
      { "field": "is_completed", "op": "EQ", "value": false },
      { "field": "due_at", "op": "LTE", "value": "@end_of_month" },
      { "field": "labels", "op": "CONTAINS_ANY", "value": ["018f...", "018f..."] },
      { "op": "OR", "nodes": [
        { "field": "assignee_id", "op": "EQ", "value": "@me" },
        { "field": "members", "op": "CONTAINS", "value": "@me" }
      ]},
      { "field": "custom_fields.priority", "op": "EQ", "value": "high" }
    ]
  },
  "sort": [ { "field": "order_key", "dir": "ASC" }, { "field": "due_at", "dir": "ASC", "nulls": "LAST" } ],
  "group_by": { "field": "bucket_id", "limit_per_group": 50 },
  "expand": ["labels"],
  "page": { "cursor": null, "size": 100 },
  "count": "none",
  "include_archived": false,
  "include_trashed": false
}
```

| Element | Rules |
|---|---|
| Scope | Required: exactly one of `container_id` and `item_id` — an unanchored query would scan the tenant — and the permission is checked against it once for the whole result. `include_descendants: false` narrows it to one level |
| Operators | `AND`, `OR`, `NOT` (exactly one node); `EQ`, `NEQ`, `IN`, `NOT_IN`, `LT`, `LTE`, `GT`, `GTE`, `BETWEEN`, `IS_NULL`, `CONTAINS`, `CONTAINS_ANY`, `CONTAINS_ALL`, `STARTS_WITH`, `MATCHES` (full text) |
| Placeholders | `@me`, `@now`, `@today`, `@end_of_day`, `@start_of_week`, `@end_of_week`, `@start_of_month`, `@end_of_month`, each but `@me` with an optional signed ISO 8601 offset (`@today+P3D`). Resolved server-side in the actor's time zone; an end is the last instant of its period; the week starts at the actor's `week_start` (the account's, else the locale's; [i18n-l10n.md](./i18n-l10n.md) §4) |
| Nesting | Maximum depth 5, maximum 50 nodes |
| Cost | An estimate over the parsed tree, capped at 50: a plain comparison costs 1, a prefix 2, a text scan (`CONTAINS`/`MATCHES`) 5, a list 1 plus one per twenty values, a `NOT` doubles its subtree. Past the cap → `422 query.filter_too_expensive` naming the estimate and the ceiling, refused before it runs ([multi-tenancy.md](./multi-tenancy.md) §4) |
| Fields | Only the fields `/meta/capabilities` lists under `query_fields`. An unknown field, or one no use case writes yet, is refused by name as `422 query.field_unknown` |
| Custom fields | `custom_fields.<key>` is recognised by its shape, the key bound as a parameter, and not listed in `query_fields`; `/custom-fields` says which keys exist, and a client offers those defined for the collection on screen. A key nothing defines matches nothing. The equalities, `IN`, `CONTAINS`, `IS_NULL`, `CONTAINS_ANY` and `CONTAINS_ALL` (a field holding several values: any or every element) and `STARTS_WITH` (a field holding one text, case-insensitive) read the document; `LT`, `LTE`, `GT`, `GTE`, `BETWEEN`, a sort and a grouping read the typed value (numbers, amounts and durations as numbers, dates as days `YYYY-MM-DD`, text by its first 256 characters). The use case reads the kinds of the definitions under the key in the query's scope first: a range over people, references or formatted text, a sort by a set, a person, a reference or formatted text, a grouping by a set or formatted text, `CONTAINS_ANY` or `CONTAINS_ALL` over a field holding one value, `STARTS_WITH` over anything but one text, and any of these over a key whose kinds disagree in the scope are refused (`query.operator_unsupported`, `query.field_not_sortable`, `query.field_not_groupable`) |
| Entry type | `entry_type` is the key of the entry's type, its level's system key (`task`, `work_package`, `activity`) when it has none of its own; `EQ`, `NEQ`, `IN`, `NOT_IN` and a grouping, no sort. `/entry-types` says which keys exist |
| Blocked | `blocked` is whether an open entry holds the entry up - over a link whose type waits - the source of a `blocks` link to it, the target of a `depends_on` link from it, or the other end of a workspace's own type as its `waits` says; `EQ` and `NEQ` with a boolean, no sort or grouping, cost 2 |
| Blocking | `blocking` is the other direction: whether the entry holds up an open one over a link whose type waits; `EQ` and `NEQ` with a boolean, no sort or grouping, cost 2 |
| Linked to | `linked_to` is whether a link of any type joins the entry to one of the given entries, from either end; `EQ` and `IN` with identifiers, no sort or grouping, cost 2 |
| Status | `status` is the key of the entry's status in the workflow its collection runs for its type, absent where none runs; `EQ`, `NEQ`, `IN`, `NOT_IN`, `IS_NULL` and a grouping, no sort - a workflow collection's board is grouped by it. `/workflows` says which keys exist |
| `group_by` | Groups, each with its own cursor, so board columns page independently; a group is continued by asking for it (its key as a filter, its cursor as the cursor). A cursor together with `group_by` is refused |
| `expand` | `labels` is served; any other relation is refused as `items.expand_not_supported` |
| Timeline | `sort=[start_at]`, filter `BETWEEN` on `start_at`/`due_at` |
| Determinism | Sorting always ends implicitly on `id ASC`, so that cursors stay stable. Without a sort, the manual order (`order_key ASC`) |

`SavedView` stores exactly this object plus `layout` and `visible_fields`, validated against the
query catalogue at the write. The server does not interpret `layout`, so a new view in a client
needs no backend change.

`POST /items:aggregate` takes the same scope and filter, with the same limits, cost estimate and
permission, and an `aggregate` instead of a sort, page or grouping: `function` (`COUNT`, `SUM`,
`AVG`, `MIN`, `MAX`), `field` (a `custom_fields.<key>` holding a number, money or a duration, or a
built-in quantity in whole minutes - `lead_time`, `created_at` to `completed_at`, and
`scheduled_span`, `start_at` to `due_at`, none where a stamp is missing; refused for `COUNT`,
`query.field_not_aggregatable` for anything else), `group_by` (a field a query may group by) and
`bucket` (`field`, a timestamp or a custom date, `period`: `day`, `week`, `month`, cut in the
actor's time zone, and for a week `week_start`: `monday`, the default, or `sunday`). Its scope may
instead be `workspace: true`: the use case asks a read of every live collection (archived ones too
with `include_archived`) at its own path, at most 2000 (`query.workspace_too_large`), and binds the
readable ones as one array, so the narrowing is inside the statement; an entry shared with the
actor alone in a collection they do not read is asked at its own path and bound as a second array,
at most 5000. `source: links` counts links instead of entries: a link whose two ends the scope and
filter both keep (the target is held to the same predicates inside the statement, so an end the
reader cannot see is never counted), `COUNT` only (`query.aggregate_links_count_only`), its
`group_by` and `bucket` reading the source end, and `group_by: link_type` its own grouping; another
source is `query.aggregate_source_unknown` - records are entries, and time records and pages have no
store yet. A saved view may store `aggregate` (and then `scope_workspace: true`
instead of an anchor); `POST /views/{viewId}:aggregate` runs it under the caller's authorisation,
and a view without one is refused (`views.aggregate_missing`); a workspace-scoped summary view has
no rows, and its `:export` is refused (`views.summary_not_exportable`). It answers at most 5000 rows of
`group`, `period`, `count` and `value` (a decimal string, so money keeps its digits), in the
group's order and then the period's, and `truncated`. A field hidden from the reader is refused as
in a filter. The function and the period are closed vocabulary like the operators; the time zone
and the limit are parameters (§3.1).

`POST /search` takes the same filter grammar, limits and cost estimate; it has no `group_by`,
`expand` or `count`. Its `words` are optional when a filter is given; without words the order is
`due_at ASC NULLS LAST, id ASC`; neither words nor a filter is refused (`search.words_required`).
`/meta/capabilities` announces this as `features.search_filter: true`, so a client that also talks
to an older server tells rather than probes.

### 3.1 How a query becomes SQL

The filter, sort and grouping of `items:query` and `/search`, and the aggregation of
`items:aggregate`, are the one place where SQL text is
assembled at run time. The boundary is **no byte that arrived in a request ever becomes SQL text**
([ADR-0026](../adr/ADR-0026-query-dsl-sql-construction.md), security.md T-06):

1. **The vocabulary is closed.** Field names, operators, sort directions, null placement and the
   grouping field are parsed into Go types in `core/domain/model/view` before the compiler sees
   them. Anything not in the catalogue is refused with `422` and a message code; it never reaches
   the adapter. `/meta/capabilities` answers `query_fields` from the same catalogue.
2. **The compiler emits only literals from the binary.** Each fragment is a constant string chosen
   by a `switch` over typed constants. The compiler package may not import `fmt` (`depguard`).
3. **Every value is a parameter** (`$n`), including the identifiers inside `IN` and the search text
   of `MATCHES`. The count of placeholders and the length of the argument slice are asserted on
   every compilation.
4. **A fuzz gate proves it:** `FuzzCompile` asserts that compiled SQL is drawn from the closed
   vocabulary, that placeholders and arguments agree, and that no fragment of the input appears in
   the SQL text (`make gate-fuzz` nightly, seeded in `make gate-unit`).
5. **sqlc keeps everything else**, this endpoint's `COUNT` included; a second hand-built statement
   is a review finding. Search and saved views reuse the same AST and compiler.

The statement runs on the unit of work's transaction under row level security, and the pool's
interactive `statement_timeout` keeps an expensive but legal filter finite. A new filterable field
is a change in two tables: the domain's catalogue and the compiler's column mapping.

---

## 4. Pagination, sorting, partial responses

* **Cursor pagination** (an opaque, signed cursor with the sort key plus `id`). No offsets.
* Response shape: `{ "data": [...], "page": { "next_cursor": "…", "has_more": true }, "groups": [...] }`.
* `size` defaults to 50, maximum 200. A larger result is read page by page, or through a bounded
  export (§5).
* Where a read answers only what the caller may see (`/search` without a container), a page can
  be shorter than the size asked for. A client reads on until `has_more` is false.
* A total is given only with `count: "exact"`, as `total`, because it costs a second pass.
  `count: "estimated"` is refused as `query.count_not_supported` rather than answered with a null
  total.

---

## 5. Write semantics

| Mechanism | Implementation |
|---|---|
| **Idempotency** | The `Idempotency-Key` header (a UUID; anything else is `422 idempotency.key_malformed`) on `POST`s; the answer is stored per tenant, key and route and replayed identically for at least 24 h — mandatory for automation and agents. `PUT` and `DELETE` repeat harmlessly, and `PATCH` is guarded by `If-Match`. Not stored, the key released for the repeat: a `5xx` (no decision the server stands behind) and `403 auth.step_up_required` (not attempted; the retry with the proof is the same intent). Stored with its status and **no body**: an answer that carries a credential shown once - a session's tokens, a token, a secret, recovery codes, a start password, an invitation's link - so no record keeps it in clear; a repeat runs nothing twice and answers the status alone, and a client that lost the first answer asks for a new credential (`rest.ShownOnceRoutes`, held to this contract by a test). Provisioning a workspace keeps its body, the owner's redemption token with it: its repeat answers the same workspace (`rest.ShownOnceKept`). A public route (`security: []`) called without a bearer accepts the key and keeps no record: there is no workspace to keep it in, and a record kept by the key alone would hand a stranger's answer to whoever learns the key - a repeat is the route's own to answer (a single-use token is spent once, an inbound delivery is a new arrival) |
| **Optimistic locking** | `ETag` on `GET`, `If-Match` on `PATCH`/`PUT`. A stale `If-Match` answers `409 version_conflict` with the current version — **not** `412`, and the same code answers a conflict found without a precondition, since the recovery is the same: re-read and reapply ([ADR-0025](../adr/ADR-0025-precondition-failures.md)). A `412` could only ever be added beside it |
| **Partial updates** | `PATCH` with JSON Merge Patch (RFC 7396); `null` deletes a field explicitly |
| **Bulk** | `POST /items:bulk` with at most 500 operations; a result per operation (`207`-like in the body, HTTP 200); `atomic: true` enforces all-or-nothing |
| **Bounded, synchronous** | `/views/{id}:export` and `/templates/{id}:instantiate` answer in the request, bounded by `max_export_rows` and `max_template_nodes` in `/meta/capabilities`; a capped export says so in `Export-Truncated` |
| **Long running** | Work that cannot be bounded to one request (a tenant's export or deletion, an audit export, a backup and its verification, a restore, an import, a re-seal, a re-index) answers `202 Accepted` with `/jobs/{id}`. Other `202`s say in the contract what they hand back |
| **Jobs** | Cancelling is cooperative: the pass stops at its next write boundary and leaves nothing in the database, though what it already put outside stays. A finished, failed or cancelled job answers `409`. A job that cannot measure progress answers `progress: null` |
| **Rate limits** | Per token and tenant; headers `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`, and `Retry-After` on `429` |

---

## 6. Error format (RFC 9457)

`Content-Type: application/problem+json`.

```json
{
  "type": "https://docs.hubtask.dev/errors/capability_not_supported",
  "title": "capability_not_supported",
  "status": 422,
  "code": "capability_not_supported",
  "detail_code": "items.cover_not_supported_for_type",
  "params": { "item_type": "ACTIVITY", "capability": "COVER" },
  "field_errors": [ { "path": "/cover", "code": "not_allowed" } ],
  "request_id": "01J9…",
  "docs": "https://docs.hubtask.dev/errors/capability_not_supported"
}
```

* `code` is stable and machine-readable (part of the contract, SemVer-relevant).
* `detail_code` plus `params` let the client localise without server-side prose
  ([ADR-0011](../adr/ADR-0011-i18n-message-codes.md)); no free text clients would have to parse. A
  `field_errors[].path` is a JSON Pointer.
* A `500 internal` carries only the code and the `request_id`; every other status, `503` included,
  keeps `detail_code` and `params`.

The standard codes are `core/domain/model/shared/Errors.go`'s, their statuses the table in
`presentation/rest/Problem.go` (`400 malformed_request`, `409 conflict|version_conflict`,
`410 gone`, `422 validation_failed|capability_not_supported`, `503 dependency_unavailable`, …). A refused query names its
problem in `detail_code` (`query.*`). A capacity quota refuses as `422` with `capacity.<quota>`
naming the quota and ceiling — waiting does not help, unlike `429`. AI that is off or out of reach is
`503 dependency_unavailable` with `ai.unavailable`.

---

## 7. Authentication at the API

The API authenticates with a bearer token, never a cookie; CORS therefore never allows credentials.
Token shapes, lifetimes and storage are in [identity.md](./identity.md) §14 and §15.

| Method | Used for | Note |
|---|---|---|
| Session access token (`hbt_sat_`) | The first-party clients | Issued by `/auth/sessions` after any sign-in method; short-lived, renewed through `/auth/sessions:refresh` with a rotating refresh token (`hbt_srt_`). A provider's own token is never an API credential |
| Personal access token (`hbt_pat_`) | Scripts, n8n, Zapier, the CLI | Scoped, stored hashed, with an expiry date |
| Service account token | Automation, AI agents | Its own actor type in the audit |
| OAuth 2 authorization code + PKCE | Third-party apps (the Zapier marketplace) | PKCE is required; consent only to scopes the catalogue names |
| Signed feed token (`hbt_cal_`) | ICS calendar | Read-only on one view, revocable |
| Personal access token as an HTTP Basic password | CalDAV clients (`/caldav/`) | The one place Basic is taken, since a calendar client can send nothing else; the user name is discarded. Outside the OpenAPI document because WebDAV's methods are not the contract's. No `MKCALENDAR`: a calendar exists because a calendar feed does |

**Scopes.** `catalogue.Scopes()` is the source, derived from the use case descriptors, so a scope no
operation checks cannot exist; `GET /meta/capabilities` and the contract list them.

* A session carries every scope except `admin:*` and `agent:destructive` (`catalogue.SessionScopes`).
  The admin surface needs a deliberately minted credential — except that a registered operator's
  session raised by a step-up carries `admin:tenants` for an hour
  ([ADR-0070](../adr/ADR-0070-the-instance-layer.md) §4). A scope a signed-in person needs is
  therefore named otherwise (`ops:read`).
* `admin:provisioning` ([multi-tenancy.md](./multi-tenancy.md) §4.3) is the purchase platform's
  subset of `admin:tenants`, minted and checked against the operator register the same way:
  provisioning, reading, finding and listing workspaces, the platform's reference on one, moving
  them between plans, suspend, resume, a deletion request, a workspace's limit exception, the use,
  the events, and the installation's subscriptions — nothing else. An operation admits it by declaring it
  beside its own scope (`usecase.Descriptor.AlsoScope`); a refusal for want of either names
  `admin:provisioning`, the narrowest that would do.
* No OAuth consent grants an `admin:*` scope or `agent:destructive` (`access.token_scope_unknown`).
* Watching and acting are separate scopes where a watching token has no business acting:
  `jobs:read` / `jobs:cancel`, `audit:read` / `audit:export`.
* Without a matching scope → `403 forbidden` with `access.insufficient_scope`, naming the scope.

---

## 8. Versioning of the interfaces

| Artefact | Rule |
|---|---|
| REST path | `/api/v1` — a new major only on a breaking change; `v1` and `v2` run in parallel for at least 12 months |
| OpenAPI document | Its own `info.version` following SemVer, tied to the release |
| Events | `….v1` in the type name; extensible additively; deprecation declared in the capability manifest |
| MCP tools | The tool name = the stable use case name; parameters additive |
| Error codes | Stable; removing one is a breaking change |
| Deprecation | The `Deprecation` (RFC 9745) and `Sunset` (RFC 8594) headers plus an entry in `/meta/capabilities` and the changelog |

**How a request field is deprecated:** in `openapi.yaml`, `deprecated: true` with
`x-deprecated-since` (the day), `x-removed-in` (the major), `x-replaced-by` (an `operationId` or
header, not prose) and, once a day is set, `x-sunset`. `make generate` reads them into the REST
adapter's table (`tools/deprecations`); the manifest lists `deprecations`, and only a request that
sends such a field is answered with `Deprecation` (and `Sunset` where a day is set). The generator
fails on a mark missing its day, version or replacement, and on a mark nothing can announce: only a
top-level property of a schema a JSON request body references can be deprecated.

Breaking changes are: removing or renaming a field, changing a type, adding a required field,
removing an enum value from *requests*, changing semantics, changing a default, removing an error
code. See [versioning-release.md](./versioning-release.md).
