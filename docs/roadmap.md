# Implementation Plan

Versioning follows [SemVer](./architecture/versioning-release.md). Before `1.0.0`, minor releases
may contain breaks; with `1.0.0`, API v1 is promised stable.

The order follows the requirement: **the core is built completely first, then automation, and the
frontend last** (its design and feature set are still open).

---

## Phase 0 — Foundation (`0.1.0`) · Walking skeleton

| Epic | Contents |
|---|---|
| Repo setup | Instantiate the template, work through the checklist in [project-structure.md](./architecture/project-structure.md) §5, `LICENSE`, `CLA`, `SECURITY.md`, `CONTRIBUTING.md` |
| Toolchain | `Makefile`, `golangci-lint`, `sqlc`, `goose`, `oapi-codegen`, architecture tests, GitHub Actions with every gate |
| Skeleton | `cmd/server` with roles, the environment port, the four health levels (`/healthz`, `/startupz`, `/readyz`, `/meta/health`), OpenTelemetry, structured logs with redaction, `SafeGo`, graceful shutdown |
| Security baseline | Gates SG-1…SG-12 in the pipeline (initially against the reference use case), `GuardedClient`, security headers, rate limits, Argon2id, token hashing, the secret type, `SECURITY.md`, secret scanning, SBOM + image signature |
| Resilience baseline | `infrastructure/resilience` (timeouts, breaker, retry, bulkhead, load shedding), panic middleware, the degradation registry, tests RT-1…RT-4 |
| Observability material | The basic metric catalogue, trace propagation across the outbox and jobs, `deploy/observability/` with a first dashboard, alert rules A-03/A-04/A-05/A-07/A-12 and their runbooks |
| Audit skeleton | `audit_log` with the hash chain and grants (no `UPDATE`/`DELETE` for the app role), the `AuditableAction` registry, gates AU-1/AU-2 on the reference use case |
| Data protection skeleton | The data catalogue created and checked in the gate, field classification, log and metric redaction, the `retention_policy` table with defaults |
| Sync foundation | `change_log`, `tombstone`, HLC generation, and position fractional indices in the data model — not introducible later without a break ([ADR-0021](./adr/ADR-0021-offline-sync.md)) |
| Persistence | The PostgreSQL connection, UnitOfWork, the tenant context (`SET LOCAL`), the RLS skeleton, the first migration |
| API skeleton | `openapi.yaml` with `/meta/*`, the generated router, problem details, middleware (auth stub, locale, request ID, rate limit) |
| Reference use case | `CreateContainer` end to end: REST + MCP + automation registration + tests + event |
| Deployment | Dockerfile (distroless, multi-arch), `compose.yaml`, the Helm chart adjusted, the migration job |

**The result:** a runnable, deployable, tested system with exactly one use case — every cross-cutting
concern driven through once, completely.

Security and reliability belong deliberately in **phase 0**, not in stabilisation: the gates must
bite while there is one use case and rework is cheap. A cross-tenant test introduced later finds
defects in 200 methods at once — one present from the start prevents them one at a time.

---

## Phase 1 — The business core (`0.2.0` – `0.4.5`)

### `0.2.0` Hierarchy and items
The identity base model (tenant, account, membership, roles, permission resolution), containers
(hub/collection), the generalised `WorkItem` with capability profiles, the hierarchy service,
buckets, labels, ordering (drag and drop), query DSL v1, trash/archive, activity history, and
`hubctl` as the first real client.

### `0.3.0` Collaboration and content
Comments, members and assignment, automatic assignment (every strategy), covers (colour/image
upload), media and attachments with presigned upload, custom fields, full-text search, notifications
(email), the SSE stream, and bulk plus duplicate — the two item operations the specification has
declared since `0.1.0` and no milestone owned.

### `0.3.5` The workspace
The repository becomes a monorepo: `apps/webapp` (the to-do application), `apps/website`
(hubtask.eu), `packages/design-system` and `packages/api-client`, plus the design tokens every
client draws from and a pipeline that runs only where a change actually landed. No use case is
added — the milestone is measured by what still works afterwards. It sits here because phase 5
starts in parallel from `0.4.0`, and the house has to exist before anybody moves in
([ADR-0027](./adr/ADR-0027-monorepo-structure.md),
[ADR-0028](./adr/ADR-0028-embedded-web-ui.md),
[ADR-0029](./adr/ADR-0029-design-system-tokens.md)).

### `0.4.0` Time
Due dates (including all-day, time zones), reminders (predefined + custom), recurrence (RRULE, both
modes, DST tests), the scheduler role, the job queue, the retention job, the ICS calendar feed,
templates, and saved views including layout hints for list/kanban/timeline.

**The result of phase 1:** the core is functionally complete per the product idea — without a
frontend.

---

### `0.4.5` Backup, retention, audit
Backup targets (`local`, `s3`, `sftp`, `webdav` first), the archive format with encryption, RRULE
schedules, generational retention, listing at the target, restore in every mode including the
deletion journal; retention rules with a preview, a grace period, and safeguards; audit query,
export, and `:verify`; legal hold; data subject requests (`data_subject_request`) with deadline
tracking and export. Further target adapters (`ftps`, `ftp`, `smb`, `azure`, `gcs`, `rclone`) follow
once they pass conformance test BK-1.

---

## Phase 2 — Automation and integration (`0.5.0` – `0.6.0`)

### `0.5.0` Automation and webhooks
The outbox dispatcher production-ready, CloudEvents schemas published, the rule engine (triggers,
CEL conditions, every action), dry run, RuleRuns, loop and throttle protection, webhook
subscriptions with HMAC/retry/dead letter, the trigger polling endpoints, PATs and service accounts
with scopes, and the jumble (email intake, webhook intake, quick capture, conversion).

### `0.6.0` Multi-tenancy and operations
Multi mode, tenant provisioning/suspension/deletion, export (GDPR), quotas and fairness, the OIDC
connection, an OAuth2 provider for third-party apps, TOTP MFA with enforcement per tenant, session
management, step-up authentication, envelope encryption with key rotation (open point S-2), role
separation in the Kubernetes deployment, HPA, PodDisruptionBudget, NetworkPolicy, load tests
(2 million items, an automation storm), partitioning, PITR backup with a verified restore (RT-9),
the complete alert catalogue A-01…A-18 with runbooks, SLO dashboards, and the optional NATS adapter.

---

## Phase 3 — AI and ecosystem (`0.7.0` – `0.9.0`)

| Version | Contents |
|---|---|
| `0.7.0` | The MCP server complete (tools, resources, prompts), the AI port with Ollama and OpenAI-compatible adapters, AI suggestions for the jumble and for decomposition, semantic search (pgvector), provenance and audit |
| `0.8.0` | i18n complete: catalogue maintenance, the Weblate connection, CLDR formats, RTL metadata, language-dependent search, localised emails; accessibility and localisation requirements for the frontend documented |
| `0.8.5` | Offline synchronisation complete: `:pull`/`:push`, per-field merging, OR-sets, fractional indices, HLC bounding, device management, conflict preservation, the SSE stream, `hubctl sync-conformance` as the reference client check |
| `0.9.0` | Ecosystem: an official n8n node and Zapier app (generated), client SDKs (TypeScript, Go, Python), CalDAV, import from Trello/Microsoft To Do/Google Tasks, public API documentation |

---

## Phase 4 — Stabilisation (`1.0.0`)

Prerequisites for `1.0.0`:

1. API v1 frozen, the OpenAPI diff clean, the deprecation process exercised.
2. Event schemas v1 stable and documented.
3. Every quality scenario QS-01 to QS-27 demonstrated (test reports in the repository).
4. Load test results against the target figures published.
5. A security review including an external pentest or code audit of the tenant boundary and the webhook/SSRF paths (open point S-1); the threat model T-01…T-20 complete with test evidence; every gate SG-1…SG-12 permanently green.
6. The upgrade path from `0.x` documented and tested.
7. Trademark registration for the name and logo completed (the licence itself is settled, [ADR-0013](./adr/ADR-0013-licensing.md)).
8. Operating documentation complete: backup, restore (with a logged drill), monitoring, the alert catalogue with a runbook per alert, an SLO report over at least 30 days, resilience tests RT-1…RT-12 green, `hubtask_panics_recovered_total` at 0 over the period.
9. Reference deployments (Compose and Helm) tested reproducibly.
10. The data catalogue `docs/privacy/data-catalog.md` and the DPA template in place (open point S-3).
11. The audit demonstrably complete and immutable: AU-1…AU-7 green, `:verify` over a production period with no findings.
12. Backup and restore verified: BK-1…BK-10 green, a documented restore drill from every released target type, and the golden archives of every major version importable.
13. Retention rules exercised: RE-1…RE-9 green, and at least one complete run of a multi-stage chain in a production environment.
14. Offline synchronisation accepted: SY-1…SY-12 green, and the conformance test passed against at least one real client.

---

## Phase 5 — Frontend (in parallel from `0.4.0`, with its own versioning)

The design and feature set are open; the backend supplies only building blocks. The place it will
live, the way it will be delivered and the values it will be drawn in are settled by `0.3.5`; what
is settled in preparation beyond that:

* The contract: an OpenAPI-generated SDK, `/meta/capabilities` for configuration, saved views with a
  `layout` hint, SSE for live updates.
* Binding requirements on every frontend: locale and time zone handling through the account
  preference, RTL support, message codes instead of server text, tolerant behaviour towards unknown
  fields, and offline-tolerant writing with an `Idempotency-Key`.
* **Offline conformance** per [offline-sync.md](./architecture/offline-sync.md) §9: client-assigned
  UUIDv7, an `op_id` per mutation, an HLC per field change, local deletion on `ACCESS_REVOKED` and
  `sync.gone`, a full resynchronisation on `sync.cursor_too_old`, and encrypted local storage with
  complete deletion on sign-out. Verifiable through `hubctl sync-conformance` — which applies to
  third-party implementations too.
* For a PWA that means concretely: local storage (IndexedDB) plus a mutation queue, a service worker
  only for delivery and background sync, and **no** conflict handling of its own — merging belongs
  on the server.
* As an interim solution, `hubctl` (the CLI) plus a minimal reference client serve for dogfooding —
  not a product decision, just a tool (risk R-08).

The choice of framework is the one thing deliberately still open, and it needs its own ADR — the
content security policy [ADR-0028](./adr/ADR-0028-embedded-web-ui.md) settles is already a
constraint on it: no `'unsafe-inline'`, no `'unsafe-eval'`. A dedicated arc42 document is created
once that decision is taken.

---

## What is immediately implementable

| Ready | Reason |
|---|---|
| The complete core (phases 0 and 1) | The domain model, invariants, use case catalogue, API contract, and persistence sketch are settled |
| The automation service (phase 2) | The rule model, trigger/action catalogue, execution semantics, and protective mechanisms are decided |
| The integration layer | The event catalogue, webhook mechanics, and auth methods are in place |
| Deployment and CI | The role model, image strategy, and pipeline gates are in place |
| The security baseline | The threat model T-01…T-20 with countermeasures and gates SG-1…SG-12 is decided ([security.md](./architecture/security.md)) |
| Audit, data protection, retention | The log model, data subject rights, deletion paths, and the retention model are decided and represented in the schema |
| Backup and restore | The target abstraction, archive format, schedules, retention, and restore modes are decided ([backup-restore.md](./architecture/backup-restore.md)) |
| Offline synchronisation | The protocol, conflict handling, and data model prerequisites are decided; the client requirements are settled ([offline-sync.md](./architecture/offline-sync.md)) |
| Observability and resilience | The health model, metric and alert catalogue, resilience patterns, and the test series RT-1…RT-12 are decided ([observability-reliability.md](./architecture/observability-reliability.md)) |
| **Not ready** | The frontend (design and feature set open), the concrete billing model, SAML/SCIM details, master key management in provider operation (S-2), and the capacity model from real load data (O-2) |
