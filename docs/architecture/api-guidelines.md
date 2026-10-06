# API Guidelines (API First)

The contract: [`../../api/openapi.yaml`](../../api/openapi.yaml) (OpenAPI 3.1). The specification
is written **before** the code; server interfaces and client SDKs are generated from it
(`oapi-codegen`). A CI job fails if the generated code and the specification drift apart
([ADR-0004](../adr/ADR-0004-api-first-openapi.md)).

---

## 1. Ground rules

1. **Nothing exists only in the UI.** Every use case is reachable through the API (coverage test against `usecase.Registry`).
2. **One major path:** `/api/v1`. Additive changes (new fields, new endpoints, new enum values in *responses*) are compatible and need no new version.
3. **Clients must be tolerant:** ignore unknown fields. This is documented in the contract.
4. **Self-describing:** `GET /api/v1/meta/capabilities` returns item types, capability profiles, field types, enum values, limits, supported locales, layout hints, query fields, automation triggers/actions, and event types. Frontends and agents configure themselves from it rather than hard-coding. An optional surface is announced under `features`; a client that does not find its flag renders what it had before and calls no route of that surface.
5. **No display text from the server** (see [i18n-l10n.md](./i18n-l10n.md)) — codes and parameters only.
6. **Consistent plural resource names**, `snake_case` for JSON fields, ISO-8601/RFC 3339 for instants, ISO-8601 durations for relative values, IANA names for time zones.

---

## 2. Resource overview

The contract lists every route; this table is the map. Paths are below `/api/v1`.

| Resource | Path | Core operations |
|---|---|---|
| Capabilities/meta | `/meta/capabilities`, `/meta/health` | `GET` |
| The workspace itself | `/tenant` | `GET`, `PATCH` — one workspace as its members see it: the display name, the two defaults every member falls back to, and the switch that demands a second factor of its administrators. Not `/admin/tenants`, which crosses workspaces and is the installation operator's |
| Installation (admin) | `/admin/tenants`, `/admin/settings`, `/admin/operators`, `/admin/identity-providers`, `/admin/encryption`, `/admin/overview`, `/admin/journal` | `GET`, `POST`; tenant actions `:suspend`, `:resume`, `:delete`, `:export`, `:open-password`, `:close-password`; `/admin/encryption:reseal` ([ADR-0045](../adr/ADR-0045-master-key-in-the-environment.md)) |
| Accounts | `/accounts:invite`, `/accounts/me`, `/accounts/{id}`, `…/preferences`, `…/notification-preferences` | `GET`, `POST`, `PATCH`, `PUT` |
| Sign-in and credentials | `/auth/sessions`, `/auth/password`, `/auth/mfa/…`, `/auth/step-up`, `/auth/tokens`, `/auth/service-accounts`, `/auth/oidc:…`, `/oauth/…`, `/identity-provider(s)` | see the contract; the rules are in [security.md](./security.md) §5 |
| Groups, memberships | `/groups`, `/memberships` | CRUD; `GET`, `POST`, `DELETE` |
| Containers (hub/collection) | `/containers` | CRUD, `:move`, `:reorder`, `:archive`, `:unarchive`, `:restore`, `PUT …/policies` |
| Items | `/items` | CRUD, `:query`, `:move`, `:reorder`, `:complete`, `:reopen`, `:duplicate`, `:bulk`, `:archive`, `:unarchive`, `:restore`, `:purge`, `:retain`, `:assign`, `:unassign`, `:auto-assign` |
| Buckets, labels | `/containers/{id}/buckets`, `/containers/{id}/labels` | CRUD; buckets also `:reorder` |
| An entry's labels, members, attachments | `/items/{id}/labels/{labelId}`, `/items/{id}/members/{accountId}`, `/items/{id}/attachments/{mediaId}` | `PUT`, `DELETE` |
| Custom fields | `/custom-fields`; values at `/items/{id}/custom-fields/{key}` | CRUD; a value is `PUT` one key per call (`null` clears), because the merge rule is per key |
| An entry's due date, cover | `/items/{id}/due`, `/items/{id}/cover` | `PUT`, `DELETE` — the due trio travels together; the same three fields on create and update dispatch into the same writer |
| Comments, history | `/items/{id}/comments`, `/items/{id}/activity` | CRUD; `GET` |
| Media | `/media`, `/media/{id}`, `:confirm`, `:content` | `POST` (presigned), `GET`, `DELETE` |
| Reminders, recurrence | `/items/{id}/reminders`, `/items/{id}/recurrence` | CRUD; `GET`, `PUT`, `DELETE`, `:skip` — a rule a client can set it can also read back |
| Templates | `/templates` | CRUD, `:instantiate`, `:generate` |
| Views | `/views` | CRUD, `:share`, `:export` |
| Search | `/search` | `POST` only. What somebody is looking for is their content, and a query string travels through access logs, proxies and browser history |
| Jumble | `/jumble/entries` | `GET`, `POST`, `:convert`, `:suggest`, `:dismiss` |
| Jumble intake | `/jumble/intake:rotate-token`, `/jumble/inbound/{token}`, `/jumble/mail/{token}` | `POST` (the address, shown once); `POST` (public, token-protected, capped); `POST` with `message/rfc822` and a bound of its own. The token authenticates the tenant rather than a person, and every reason not to serve answers the same `404` |
| Automation | `/automation/rules`, `/automation/runs`, `/automation/inbound/{token}` | CRUD, `:enable`, `:disable`, `:check`, `:test`, `:trigger`, `:rotate-inbound-token`, `:replay` |
| Webhooks, outbound calls | `/integrations/webhooks`, `…/deliveries`, `/integrations/http-requests` | CRUD, `:send`, `:replay`, `:rotate-secret` |
| Trigger polling (Zapier/n8n) | `/integrations/triggers/{eventType}` | `GET` (sorted by `since`/cursor, deduplicable) |
| Calendar | `/integrations/calendar-feeds`, `/calendar/{token}.ics`, `/caldav/` | `GET`, `POST`, `DELETE`; `GET` (public, token-protected); WebDAV outside the contract (§7) |
| AI | `/ai-provider`, `/items/{id}:decompose` and the other assistance actions, `/suggestions` | `GET`, `PUT`, `DELETE`; `POST`; `GET`, `:accept`, `:dismiss` ([ai-first.md](./ai-first.md)) |
| Backup and restore | `/backup-targets`, `/backup-schedules`, `/backups`, `/restores` | `GET`, `POST`, `PATCH`, `DELETE`, `:test`, `:verify`. Deleting a target a schedule still names is a `409`; nothing at the target is ever touched |
| Retention, trash, holds | `/retention-policies`, `/trash`, `/legal-holds` | `GET`, `POST`, `PATCH`, `DELETE`, `:preview`; `GET`, `:empty`; `:release` |
| Audit | `/audit`, `/audit:verify`, `/audit:export`, `/audit/anchoring` | `GET`, `POST`, `PUT` |
| Privacy | `/privacy/requests`, `/privacy/consents:withdraw`, `/accounts/{id}:restrict` | `GET`, `POST`, `PATCH` |
| Imports, quotas | `/imports`, `/quotas` | `POST`, `GET` |
| Jobs | `/jobs/{id}`, `/jobs/{id}:cancel` | `GET`, `POST` (§5) |
| Sync | `/sync:snapshot`, `/sync:pull`, `/sync:push`, `/sync/devices` | `POST`; `GET`, `DELETE` ([offline-sync.md](./offline-sync.md)) |
| Event stream | `/stream` (SSE) | `GET` |
| MCP | `/mcp` | Streamable HTTP, outside the OpenAPI document |

**Actions** use the suffix pattern `POST /items/{id}:complete` (Google AIP style) — clearer than
status fields for operations with side effects, and easier for agents to understand.

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
| Scope | Required: exactly one of `container_id` and `item_id`. An unanchored query is a scan of the whole tenant, and the permission is checked against the scope once for the whole result. `include_descendants: false` narrows it to one level |
| Operators | `AND`, `OR`, `NOT` (exactly one node); `EQ`, `NEQ`, `IN`, `NOT_IN`, `LT`, `LTE`, `GT`, `GTE`, `BETWEEN`, `IS_NULL`, `CONTAINS`, `CONTAINS_ANY`, `CONTAINS_ALL`, `STARTS_WITH`, `MATCHES` (full text) |
| Placeholders | `@me`, `@now`, `@today`, `@end_of_day`, `@start_of_week`, `@end_of_week`, `@start_of_month`, `@end_of_month`, each but `@me` with an optional signed ISO 8601 offset (`@today+P3D`). Resolved server-side in the actor's time zone; an end is the last instant of its period; the week starts at the actor's `week_start` (the account's, else the locale's; [i18n-l10n.md](./i18n-l10n.md) §4) |
| Nesting | Maximum depth 5, maximum 50 nodes |
| Cost | An estimate over the parsed tree, capped at 50: a plain comparison costs 1, a prefix 2, a text scan (`CONTAINS`/`MATCHES`) 5, a list 1 plus one per twenty values, a `NOT` doubles its subtree. Past the cap → `422 query.filter_too_expensive` naming the estimate and the ceiling, refused before it runs ([multi-tenancy.md](./multi-tenancy.md) §4) |
| Fields | Only the fields `/meta/capabilities` lists under `query_fields`. An unknown field, or one no use case writes yet, is refused by name as `422 query.field_unknown` |
| Custom fields | `custom_fields.<key>` is recognised by its shape, with the key bound as a parameter, and is not listed in `query_fields`; which keys exist is `/custom-fields`' answer. A client offers each key the definitions name for the collection on screen, with the comparisons its kind takes. A key nothing defines matches nothing |
| `group_by` | Returns groups, each with its own cursor, so board columns page independently. A group is continued by asking for that group: its key as a filter, its cursor as the cursor. A cursor together with `group_by` is refused |
| `expand` | `labels` is served; any other relation is refused as `items.expand_not_supported` |
| Timeline | `sort=[start_at]`, filter `BETWEEN` on `start_at`/`due_at` |
| Determinism | Sorting always ends implicitly on `id ASC`, so that cursors stay stable. Without a sort, the manual order (`order_key ASC`) |

`SavedView` stores exactly this object plus `layout` and `visible_fields`. A saved query is
validated against the query catalogue when it is written. The server does not interpret
`layout`, so a new view in a client needs no backend change.

`POST /search` takes the same filter grammar, limits and cost estimate; it has no `group_by`,
`expand` or `count`.

### 3.1 How a query becomes SQL

The filter, sort and grouping of `items:query` and `/search` are the one place where SQL text is
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
5. **sqlc keeps everything else**, this endpoint's `COUNT` included. A second hand-built statement
   anywhere else is a review finding. Search and saved views reuse the same AST and compiler.

The compiled statement runs on the unit of work's transaction, under row level security, and the
pool's interactive `statement_timeout` keeps an expensive but legal filter finite. Adding a
filterable field is a change in two tables: the catalogue in the domain and the column mapping in
the compiler.

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
| **Idempotency** | The `Idempotency-Key` header (a UUID; anything else is `422 idempotency.key_malformed`) on `POST`s; the answer is stored per tenant, key and route and replayed identically on a repeat for at least 24 h — mandatory for automation and agent use. `PUT` and `DELETE` repeat harmlessly by definition, and `PATCH` is guarded by `If-Match`. Two answers are not stored, and the key is released for the repeat instead: a `5xx` (not a decision the server stands behind) and `403 auth.step_up_required` (the request was not attempted for want of a proof — the retry carrying the proof is the same intent under the same key) |
| **Optimistic locking** | `ETag` on `GET`, `If-Match` on `PATCH`/`PUT`. A stale `If-Match` answers `409 version_conflict` with the current version in the payload — **not** `412`, and the same code answers a conflict found without a precondition, because the client's recovery is the same: re-read and reapply ([ADR-0025](../adr/ADR-0025-precondition-failures.md)). A `412 precondition_failed` could only ever be added beside it, never instead |
| **Partial updates** | `PATCH` with JSON Merge Patch (RFC 7396); `null` deletes a field explicitly |
| **Bulk** | `POST /items:bulk` with at most 500 operations; the response contains a result per operation (`207`-like in the body, HTTP 200), and `atomic: true` enforces all-or-nothing |
| **Bounded, synchronous** | `/views/{id}:export` and `/templates/{id}:instantiate` answer in the request. Each is bounded by a limit in `/meta/capabilities` (`max_export_rows`, `max_template_nodes`); an export that reached its cap says so in the `Export-Truncated` header |
| **Long running** | Work that cannot be bounded to one request — a tenant's export or deletion, an audit export, a backup and its verification, a restore, an import, a re-seal, a re-index — answers `202 Accepted` with `/jobs/{id}`. Other `202`s (an AI suggestion, a triggered rule, a webhook send) say in the contract what they hand back |
| **Jobs** | Cancelling is cooperative: the pass stops at its next write boundary and leaves nothing behind in the database, though what it already put outside (bytes at a backup target, a handed-off mail) stays. A job already finished, failed or cancelled answers `409`. A job that cannot measure its progress answers `progress: null` rather than an invented number |
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

* `code` is stable and machine-readable (part of the contract, and SemVer-relevant).
* `detail_code` plus `params` let the client produce a localised message without any server-side
  prose ([ADR-0011](../adr/ADR-0011-i18n-message-codes.md)). A `field_errors[].path` is a JSON
  Pointer.
* No free text that clients would have to parse.
* A `500 internal` carries no `detail_code` and no `params` — only the code and the `request_id`.
  Every other status, `503` included, keeps them.

The standard mapping (`presentation/rest/Problem.go` is the table): `400 malformed_request`,
`401 unauthenticated`, `403 forbidden`, `404 not_found`, `405 method_not_allowed`,
`409 conflict|version_conflict`, `410 gone` (permanently deleted), `413 payload_too_large`,
`422 validation_failed|capability_not_supported`, `429 rate_limited`, `500 internal`,
`503 dependency_unavailable`. A refused query names its problem in `detail_code` (`query.*`). A
capacity quota refuses as `422` with `capacity.<quota>` naming the quota and the ceiling — waiting
does not help, which is what separates it from `429`. AI that is off or out of reach is
`503 dependency_unavailable` with `ai.unavailable`.

---

## 7. Authentication at the API

The API authenticates with a bearer token and never with a cookie; CORS therefore never allows
credentials. Token shapes, lifetimes and storage are in [security.md](./security.md) §5.

| Method | Used for | Note |
|---|---|---|
| Session access token (`hbt_sat_`) | The first-party clients | Issued by `/auth/sessions` after any sign-in method, including a provider's; short-lived, renewed through `/auth/sessions:refresh` with a rotating refresh token (`hbt_srt_`). A provider's own token is never an API credential |
| Personal access token (`hbt_pat_`) | Scripts, n8n, Zapier, the CLI | Scoped, stored hashed, with an expiry date |
| Service account token | Automation, AI agents | Its own actor type in the audit |
| OAuth 2 authorization code + PKCE | Third-party apps (the Zapier marketplace) | PKCE is required; consent is given only to scopes the catalogue names |
| Signed feed token (`hbt_cal_`) | ICS calendar | Read-only on one view, revocable |
| Personal access token as an HTTP Basic password | CalDAV clients (`/caldav/`) | The one place Basic is taken, because a calendar client can send nothing else; the user name is read and discarded, the password is the token. The tree is outside the OpenAPI document because WebDAV's methods are not the contract's. It offers no `MKCALENDAR`: a calendar exists because a calendar feed does |

**Scopes.** `catalogue.Scopes()` is the source: it is derived from the use case descriptors, so a
scope no operation checks cannot exist. `GET /meta/capabilities` and the contract list them; this
document does not keep a copy. The rules:

* A session carries every scope except `admin:*` and `agent:destructive` (`catalogue.SessionScopes`).
  The admin surface is entered by a deliberately minted credential — with one exception: a
  registered operator's session raised by a step-up carries `admin:tenants` for an hour
  ([ADR-0070](../adr/ADR-0070-the-instance-layer.md) §4). A scope a signed-in person needs is
  therefore named something else (`ops:read`).
* Watching and acting are separate scopes where a token minted to watch has no business acting:
  `jobs:read` / `jobs:cancel`, `audit:read` / `audit:export`.
* Without a matching scope → `403 forbidden` with `access.insufficient_scope`, naming the scope
  required.

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
`x-deprecated-since` (the day), `x-removed-in` (the major version it goes with), `x-replaced-by` (the
`operationId` or header that takes its place — identifiers, not prose; the field's description says
the rest) and, once a day is set, `x-sunset`. `make generate` reads them into the REST adapter's
table (`tools/deprecations`); from it the manifest lists `deprecations`, and a request that sends
such a field is answered with `Deprecation` and, where a day is set, `Sunset`. A request that does
not send it hears nothing. A mark without its day, version or replacement fails the generator, and
so does a mark nothing can announce — a deprecated parameter, operation, nested or response-only
member: only a top-level property of a schema a JSON request body references is.

Breaking changes are: removing or renaming a field, changing a type, adding a required field,
removing an enum value from *requests*, changing semantics, changing a default, removing an error
code. See [versioning-release.md](./versioning-release.md).
