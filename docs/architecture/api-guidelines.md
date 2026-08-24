# API Guidelines (API First)

The contract: [`../../api/openapi.yaml`](../../api/openapi.yaml) (OpenAPI 3.1). The specification
is written **before** the code; server interfaces and client SDKs are generated from it
(`oapi-codegen`). A CI job fails if the generated code and the specification drift apart.

---

## 1. Ground rules

1. **Nothing exists only in the UI.** Every use case is reachable through the API (coverage test against `usecase.Registry`).
2. **One major path:** `/api/v1`. Additive changes (new fields, new endpoints, new enum values in *responses*) are compatible and need no new version.
3. **Clients must be tolerant:** ignore unknown fields. This is documented in the contract.
4. **Self-describing:** `GET /api/v1/meta/capabilities` returns item types, capability profiles, field types, enum values, limits, supported locales, layout hints, automation triggers/actions, and event types. Frontends and agents should configure themselves from it rather than hard-coding.
5. **No display text from the server** (see [i18n-l10n.md](./i18n-l10n.md)) — codes and parameters only.
6. **Consistent plural resource names**, `snake_case` for JSON fields, ISO-8601/RFC 3339 for instants, ISO-8601 durations for relative values, IANA names for time zones.

---

## 2. Resource overview

| Resource | Path | Core operations |
|---|---|---|
| Capabilities/meta | `/meta/capabilities`, `/meta/health` | `GET` |
| Tenants (admin) | `/admin/tenants` | `GET`, `POST`, `PATCH`, `DELETE`, `POST :export` |
| Accounts | `/accounts`, `/accounts/me` | `GET`, `POST`, `PATCH` |
| Groups | `/groups` | CRUD |
| Memberships | `/memberships` | `GET`, `POST`, `DELETE` |
| Containers (hub/collection) | `/containers` | CRUD, `:move`, `:archive`, `:unarchive`, `:restore` |
| Items | `/items` | CRUD, `:query`, `:move`, `:complete`, `:reopen`, `:reorder`, `:duplicate`, `:bulk`, `:archive`, `:restore`, `:assign`, `:unassign`, `:auto-assign` |
| Buckets | `/containers/{id}/buckets` | CRUD, `:reorder` |
| Labels | `/containers/{id}/labels` | CRUD |
| An entry's labels | `/items/{id}/labels/{labelId}` | `PUT`, `DELETE` |
| An entry's members | `/items/{id}/members/{accountId}` | `PUT`, `DELETE` |
| Custom fields | `/custom-fields` | CRUD |
| An entry's custom field values | `/items/{id}/custom-fields/{key}` | `PUT` (null clears; one key per call, because the merge rule is per key) |
| Comments | `/items/{id}/comments` | CRUD |
| History | `/items/{id}/activity` | `GET` |
| Attachments/media | `/media`, `/media/{id}` | `POST` (presigned), `GET`, `DELETE` |
| Reminders | `/items/{id}/reminders` | CRUD |
| Recurrence | `/items/{id}/recurrence` | `PUT`, `DELETE`, `POST :skip` |
| Templates | `/templates` | CRUD, `:instantiate` |
| Views | `/views` | CRUD, `:share`, `:export` |
| Jumble | `/jumble/entries` | `GET`, `POST`, `:convert`, `:dismiss` |
| Trash/archive | `/trash`, `/archive` | `GET`, `:restore`, `:purge` |
| Automation | `/automation/rules`, `/automation/runs` | CRUD, `:test`, `:trigger`, `:replay` |
| Webhooks | `/integrations/webhooks`, `/integrations/webhooks/{id}/deliveries` | CRUD, `:replay`, `:rotate-secret` |
| Trigger polling (Zapier/n8n) | `/integrations/triggers/{eventType}` | `GET` (sorted by `since`/cursor, deduplicable) |
| Calendar | `/integrations/calendar-feeds`, `/calendar/{token}.ics` | CRUD, `GET` (public, token-protected) |
| Tokens | `/auth/tokens` (PAT), `/auth/service-accounts` | `GET`, `POST`, `DELETE` |
| Search | `/search` | `GET`/`POST` |
| Event stream | `/stream` (SSE) | `GET` |
| MCP | `/mcp` | Streamable HTTP |

**Actions** use the suffix pattern `POST /items/{id}:complete` (Google AIP style) — clearer than
status fields for operations with side effects, and easier for agents to understand.

---

## 3. The query DSL (the basis of every view)

One endpoint serves list, kanban, and timeline: `POST /api/v1/items:query`.

```json
{
  "scope": { "container_id": "018f...", "include_descendants": true },
  "filter": {
    "op": "AND",
    "nodes": [
      { "field": "type", "op": "IN", "value": ["TASK"] },
      { "field": "is_completed", "op": "EQ", "value": false },
      { "field": "due_at", "op": "LTE", "value": "2026-08-31T23:59:59Z" },
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
  "expand": ["children:1", "labels", "assignee", "cover"],
  "page": { "cursor": null, "size": 100 },
  "include_archived": false,
  "include_trashed": false
}
```

| Element | Rules |
|---|---|
| Operators | `EQ`, `NEQ`, `IN`, `NOT_IN`, `LT`, `LTE`, `GT`, `GTE`, `BETWEEN`, `IS_NULL`, `CONTAINS`, `CONTAINS_ANY`, `CONTAINS_ALL`, `STARTS_WITH`, `MATCHES` (full text) |
| Placeholders | `@me`, `@today`, `@start_of_week`, `@end_of_month`, relative durations (`@today+P3D`) — resolved server-side in the actor's time zone |
| Nesting | Maximum depth 5, maximum 50 nodes (protection against expensive queries) |
| Fields | Only fields from `/meta/capabilities`; unknown fields → `422 invalid_query_field` |
| `group_by` | Returns groups each with their own cursor → kanban columns can be paged independently |
| Timeline | `sort=[start_at]`, filter `BETWEEN` on `start_at`/`due_at` |
| Lists expanded/collapsed | `expand=children:0` or `children:2` — the same query, a different depth |
| Determinism | Sorting always ends implicitly on `id ASC`, so that cursors stay stable |

`SavedView` stores exactly this object plus `layout` and `visible_fields`. The server does not
interpret `layout` — which means new views in the frontend are possible at any time without a
backend change (requirement C-14).

---

## 4. Pagination, sorting, partial responses

* **Cursor pagination** (an opaque, signed cursor with the sort key plus `id`). No offsets for large sets.
* Response shape: `{ "data": [...], "page": { "next_cursor": "…", "has_more": true }, "groups": [...] }`.
* `size` defaults to 50, maximum 200 (bulk export goes through `:export` jobs).
* `fields=` allows sparse fieldsets for mobile clients.
* Total counts are expensive: `X-Total-Count` only with `?count=exact`, otherwise `count=estimated`.

---

## 5. Write semantics

| Mechanism | Implementation |
|---|---|
| **Idempotency** | The `Idempotency-Key` header (a UUID) on all `POST`s; the result is stored for 24 h and returned identically on a repeat — mandatory for automation and agent use |
| **Optimistic locking** | `ETag` on `GET`, `If-Match` on `PATCH`/`PUT`; a conflict → `409 version_conflict` with the current version in the payload |
| **Partial updates** | `PATCH` with JSON Merge Patch (RFC 7396); `null` deletes a field explicitly |
| **Bulk** | `POST /items:bulk` with at most 500 operations; the response contains a result per operation (`207`-like in the body, HTTP 200), and `atomic: true` enforces all-or-nothing |
| **Long running** | `:export`, `:instantiate` of large templates, tenant deletion → `202 Accepted` + `/jobs/{id}` |
| **Rate limits** | Per token and tenant; headers `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`, and `Retry-After` on `429` |

---

## 6. Error format (RFC 9457)

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
* `detail_code` plus `params` let the client produce a localised message without any server-side prose.
* No free text that clients would have to parse.

The standard mapping: `400 malformed_request`, `401 unauthenticated`, `403 forbidden`,
`404 not_found`, `409 conflict|version_conflict`, `410 gone` (permanently deleted),
`422 validation_failed|capability_not_supported|invalid_query_field`,
`429 rate_limited`, `500 internal`, `503 dependency_unavailable|ai_unavailable`.

---

## 7. Authentication at the API

| Method | Used for | Note |
|---|---|---|
| OIDC access token (bearer) | Web and mobile clients | Short-lived, refreshed through the IdP |
| Session cookie (optional) | The first-party web UI | `SameSite=Lax`, a CSRF token, only if the frontend needs it |
| Personal access token | Scripts, n8n, Zapier (phase 1) | Prefix `hbt_pat_`, scoped, stored hashed, with an expiry date |
| Service account token | Automation, AI agents | Its own actor type in the audit |
| OAuth2 authorization code + PKCE | Third-party apps (from milestone 0.6) | Required for the Zapier marketplace |
| Signed feed token | ICS calendar | Read-only on one view, revocable |

Scopes: `items:read`, `items:write`, `containers:read`, `containers:write`, `comments:write`,
`automation:read`, `automation:manage`, `webhooks:manage`, `views:manage`, `media:write`,
`admin:tenants`, `ai:use`. Without a matching scope → `403 insufficient_scope`, naming the scope
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
| Deprecation | The `Deprecation` and `Sunset` headers (RFC 8594) plus an entry in `/meta/capabilities` and the changelog |

Breaking changes are: removing or renaming a field, changing a type, adding a required field,
removing an enum value from *requests*, changing semantics, changing a default, removing an error
code. See [versioning-release.md](./versioning-release.md).
