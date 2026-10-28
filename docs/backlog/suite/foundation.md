# The foundation — how Hubtask's core changes so the suite stands on it

Status: **accepted by the owner's word, 2026-10-28** (the owner worked the concept out with the
worker and accepts it as a whole; the earlier questions D-01 … D-18 are recorded as decided in
[`../decisions/accepted.md`](./accepted.md)). Evidence: the code audit (§1), the research
in [`../research/`](./research/README.md).

## 1. Where the core stands (audit of `main` b832f6ceb, 2026-10-28)

**Strong and generic — keep:** one use-case registry as the only door (authorisation, audit,
activity, events, change log, metrics, entitlement once; sync push and AI acceptance run through
it); RLS with `FORCE`, one transaction wrapper, tenant-composite keys, narrow `SECURITY DEFINER`
doors; transactional outbox, Postgres job queue, idempotent consumers, CloudEvents with versioned
schemas; CEL behind a port with cost and time bounds; the query compiler's closed vocabulary
(ADR-0026); server-authoritative sync with per-field HLC and its conformance suite; "narrowing
hides, never deletes"; the gate culture (selftest, api-compat, contract, cross-tenant negatives,
SBOM, cosign, provenance); message codes, tokens, no content in logs; hexagonal boundaries.

**Not generic — the cost of the suite sits here:**

| # | Finding | Evidence |
|---|---|---|
| A-1 | Types are a Postgres enum `item_type` with CHECKs welded to `TASK`; "a new type is configuration" (`domain-model.md` §2) is false at the database | `db/schema.sql` ~L908 |
| A-2 | Custom fields: 8 kinds, JSONB with one GIN index; filters only equality, no range, sort, group | `infrastructure/postgres/query/Compiler.go`, `core/domain/model/view/Field.go` |
| A-3 | No relation between entries except parent/child | schema |
| A-4 | Completion is a boolean and mandatory; buckets stand in for statuses | `domain-model.md` §2, §3.5 |
| A-5 | Two settings systems, hand-mapped per area; no hub/collection/person level for most settings | `InstanceValues.go`, `StructureSettings.go` |
| A-6 | REST controllers (~170) and `hubctl` (~60 commands) hand-written beside a 22k-line contract; only MCP and automation derive from the registry; permission is called inside handlers, not declared | `presentation/rest`, `cmd/hubctl`, `Authorization.go` |
| A-7 | Fixed roles, 8 permissions in a map | `core/domain/service/Authorization.go` |
| A-8 | Sync: 7 item-centric mutation kinds, hand-wired per entity; no realtime document channel (SSE only) | `core/domain/model/sync/Mutation.go`, `InitialSync.go` |
| A-9 | Query DSL: one entity, one container scope, no aggregation, no traversal; search covers item title and notes only | `QuerySpec.go`, ADR-0034 |
| A-10 | `work_item` has ~57 columns and ~20 indexes; capacity measured at 5,000 items; no per-tenant job fairness; no durable timers; frontend imports every view statically | `observability-reliability.md` §13.2, `App.svelte` |

## 2. The target — thirteen foundation changes

Each is built inside the existing rules (hexagonal core, rule 1–15, registry as the only door,
RLS, expand/contract migrations, contract first). Effort: S < M < L < XL.

| # | Change | Replaces | Effort |
|---|---|---|---|
| **F-1** | **Type registry.** `entry_type` rows per workspace (key, level, behaviours: *completable*, *workflow*, *record*, *schedulable*, *estimable*; allowed children; form; workflow). The three system types become seeded rows; the enum and the `TASK` CHECKs go by expand/contract. Records (CMDB CIs, risks, contacts) live in the same store without completion | A-1, A-4 | XL |
| **F-2** | **Typed fields.** A field-definition registry with extensible kinds (adds money, duration, reference, user set, calculated, rich text) and a typed side table `entry_field_value (tenant, entry, field, num, ts, text, ref)` with B-tree indexes for range, sort and aggregation; the JSONB document stays as the read projection, written in the same transaction | A-2 | L |
| **F-3** | **Links and references.** `entry_link` with workspace-defined link types (blocks, duplicates, depends on, runs on, affects, implements; direction, cardinality, cycle rule); a polymorphic **reference** (`kind, id, anchor`) used by mentions, cards, embeds, backlinks across entries, pages, boards, views, goals (P-19). Permission per reference resolved for the reader; a reference never leaks a title | A-3 | L |
| **F-4** | **Forms and workflows in the write path.** Form (layout + CEL logic) and workflow (statuses with categories, steps with conditions, checks, asks, then-rules) as domain services; one generalised `EntryGate` evaluates them **synchronously inside every writing use case**, so REST, MCP, `hubctl`, rules, sync push, import and mail meet the same refusal (P-18). Typed CEL environments per type; a client-side evaluator for offline forms. Completion derived from the status category; buckets become a view over statuses | A-4 | XL |
| **F-5** | **One settings catalogue.** `SettingDefinition` registry (key, kind, bounds, levels installation → plan → workspace → hub → collection → person, set/lock/restrict, writer permission, preview); one resolver answering `{effective, source, locked_by, allowed}`; one storage shape; screens generated from declarations. Today's installation and structure settings migrate with golden tests on their resolution | A-5 | L |
| **F-6** | **Configuration document.** Stable keys for every configuration object; `config_revision`; schema, export, explain, validate, plan, apply, history, rollback on REST, `hubctl`, MCP; agent plans waiting for approval; versioned templates on top (three-way upgrade) | A-5 | L |
| **F-7** | **Registry-driven doors and roles.** Descriptors gain typed output and a declared permission; REST mapping and `hubctl` commands generated from descriptors and the contract; custom roles from a permission catalogue (field and step permissions included); a requester principal for the portal | A-6, A-7 | L |
| **F-8** | **Generic sync and a realtime document service.** Entity-generic mutations (`ENTITY_UPSERT/PATCH/DELETE` with declared merge rules per entity); a websocket service for CRDT documents (pages, whiteboards): Yjs-compatible update log per document, snapshots and compaction, awareness, per-document authorisation, retention and erasure of history | A-8 | XL |
| **F-9** | **Query v2 and a search port.** Multi-entity sources (entries, records, links, time, pages), workspace-wide scope narrowed in the statement, aggregations and time buckets for dashboards, link traversal, typed field operators; a search port with the Postgres adapter first and an external index as an option; indexes comments, fields, pages, attachments | A-9 | L |
| **F-10** | **Scale and fairness.** Slim `work_item` (retention, announcement, flow state to side tables); hash partitioning prepared; per-tenant fair job claiming; durable timers on business calendars (SLA, approvals, effectiveness checks); DB query metrics; capacity runs at 10⁶ and 10⁷ entries; route-level code splitting and lazy editors in the web app | A-10 | L |
| **F-11** | **Connector framework and reconciliation.** Generalise the importer and `GuardedClient` into a connector SPI (pull, push intake, file; schedule, mapping, dry run, rate limits, sealed credentials); an identification-and-reconciliation engine for records (identification rules, `external_ref(source, id)`, source precedence per attribute, staleness takeover, duplicate tasks) — the one write path every connector uses | — | L |
| **F-12** | **Governed agents.** An agent registry (identity acting on someone's behalf, role, scopes, budget, kill switch), approval of plans and destructive acts, evaluation runs, cost and quota metering, AI-Act transparency marks on agent acts and AI content, MCP on the current specification with agent-to-agent hand-off as an adapter | — | M |
| **F-13** | **Compliance by design.** CRA: coordinated vulnerability disclosure, 24 h / 72 h / 14 d reporting process, SBOM per release, CSAF/VEX advisories, `security.txt`, a published support period; AI Act Art. 50 marks, and a design rule that no AI assigns work by a person's behaviour or scores performance; Data Act / DORA / GDPR: full workspace export **and** import; EN 301 549 v4.1.1 (WCAG 2.2 AA); NIS2 supplier pack (SBOM, CVD, patch timelines); ISO 27001 / BSI C5 control mapping; product-liability-ready release records | — | M |

Not changed: one image and PostgreSQL as the minimum (P-09); the realtime service and an external
search index are optional roles of the same binary; NATS stays optional.

## 3. New third-party dependencies this concept names (accepted with it)

| Dependency | For | Where (rule 1: never in `core/domain`, `core/port`) |
|---|---|---|
| A Yjs-compatible CRDT implementation for the browser (Yjs) and its update format on the server (stored and relayed; merged in a worker) | F-8 pages, whiteboards | `apps/`, `packages/`, `infrastructure/realtime` |
| A block editor on ProseMirror (TipTap or BlockNote core, MIT parts only — no GPL/AGPL export packages) | pages | `packages/` |
| A canvas engine for whiteboards (tldraw SDK licence checked against Apache-2.0 distribution; else Excalidraw, MIT) | whiteboards | `packages/` |
| A CEL evaluator for the browser (cel-js or the Go CEL compiled to WebAssembly) | F-4 offline forms | `packages/` |
| A websocket library for Go | F-8 | `infrastructure/realtime` |

Each is checked by the existing licence gate (no copyleft) when it lands; a failing one is replaced,
not excepted.

## 4. Order — the foundation before the modules

1. F-5 settings catalogue · F-7 registry-driven doors (they shape every later change)
2. F-1 type registry · F-2 typed fields · F-3 links and references
3. F-4 forms and workflows · F-6 configuration document and templates
4. F-9 query v2 and search · F-10 scale and fairness
5. F-8 realtime documents · F-11 connectors and reconciliation · F-12 governed agents
6. F-13 runs alongside from the first step (process and release work, not a phase)
