# Engineering Guidelines

Complements arc42 chapters 10 and 11. Binding for all contributions.

---

## 1. Test strategy

| Level | Scope | Tooling | Runtime budget |
|---|---|---|---|
| **Domain** | Invariants, state transitions, capability rules, hierarchy, recurrence, assignment strategies | Pure table tests, no mocks, no database | < 10 s in total |
| **Application** | Use cases with in-memory fakes of the ports; permissions, idempotency, event emission | Standard `testing`, fakes in the repository | < 30 s |
| **Infrastructure** | Repositories, RLS, queue, outbox, migrations | Testcontainers with a real PostgreSQL | < 5 min |
| **Contract** | REST responses against `openapi.yaml`, events against JSON Schema, MCP tool schemas | Schema validation in the test | < 1 min |
| **End-to-end** | The core paths over HTTP against the complete process (Compose) | `hubctl` + Go tests | < 5 min |
| **Load** | Query DSL over 2 million items, automation storm, webhook backlog | k6 or Vegeta, a data generator | Nightly / before a release |
| **Architecture** | Import and layer rules, use case parity | `go-arch-lint`/`depguard` plus a custom registry test | < 10 s |

**Mandatory test cases with a history of going wrong** (golden files):
DST transitions in both directions for several time zones, leap year / 29 February in RRULE,
`FREQ=MONTHLY;BYMONTHDAY=31`, an account changing time zone with existing reminders,
Unicode titles (emoji, RTL, combining characters) in length checks and search,
cross-tenant negative tests for every repository, moving a subtree across collection boundaries,
an automation loop running to the abort depth, partial failure in a bulk operation.

No tests against randomness or the system clock: `Clock`, `IDGenerator`, and `RandomSource` are
injected.

---

## 2. Definition of Ready (the story may be implemented)

1. The bounded context and aggregate are named.
2. The affected use cases are named (using the names from the catalogue in [domain-model.md](./domain-model.md)).
3. The API impact is settled: new/changed operations, field names, error codes; if it breaks something → an ADR.
4. Domain events are named (new/changed) including a compatibility assessment.
5. Permissions are defined: which role, which scope may do this.
6. i18n: the required message codes are named; no display text in the backend.
7. Tenancy: the `tenant_id` path and the RLS impact have been checked.
8. Automatability: provided for as an automation action and/or trigger.
9. The migration need and the expand/contract steps are sketched out.
10. Acceptance criteria are formulated testably (including error cases).
11. **Security assessment**: new attack surface named, the affected threats (T-xx) from [security.md](./security.md) checked, new threats added there.
12. **Failure behaviour** named: which dependency is touched, what happens when it fails, which feature degrades and how ([observability-reliability.md](./observability-reliability.md) §7).
13. **Data protection assessment**: new personal data fields added to the data catalogue, purpose and retention named, deletion path defined ([data-protection.md](./data-protection.md)).
14. **Audit obligation** settled: is the operation security- or compliance-relevant? If so, enter the action in the `AuditableAction` registry.
15. **Sync impact** settled: does the change produce change log entries, and how is the field merged on offline conflicts (LWW, OR-set, fractional index, server-side)? ([offline-sync.md](./offline-sync.md) §4)
16. **Client availability** named: which area of the client capability matrix the feature belongs to (end-user, profile configuration, administration); a restriction beyond [ADR-0032](../adr/ADR-0032-client-capability-matrix.md)'s matrix needs its justification recorded there via supersede.

## 3. Definition of Done

1. Code and tests green at every relevant level; coverage thresholds held.
2. `openapi.yaml` updated, code generation run, no diff after `make generate`.
3. The use case is registered in the registry → available via REST, MCP, and automation (parity test green).
4. Event schemas added under `api/events/`.
5. A migration exists, is safe for rolling updates, and has been tested against the previous state.
6. Message codes added to `locales/en.json`.
7. Permissions and negative tests exist (including cross-tenant).
8. Observability: a metric **and** a trace span per use case (gate RT-12), error classification set, logs free of user content and secrets.
9. Resilience: every call has a timeout/deadline, concurrency goes only through `SafeGo`, external effects go through the outbox or jobs, idempotency is assured, and the failure behaviour of the touched dependency has been tested.
10. Security: authorisation in the application layer, a cross-tenant negative test added, outbound calls through `GuardedClient`, the affected SG-x gates green.
11. Audit and data protection: the auditable action is registered and tested (gate SG-13), new fields are in the data catalogue with a deletion path, and no user content appears in the audit, logs, or metrics.
12. Retention and sync: the new data kind is added to the retention catalogue, a merge rule is defined for every new field, and the archive format and import path are adjusted for model changes (BK-4).
13. Documentation updated (the arc42 section, an ADR for an architectural decision, the changelog via the commit).
14. The Conventional Commit title is correct; a breaking change is marked.
15. **Client impact** settled: a change to `api/openapi.yaml` carries its client fix in the same pull request — `packages/api-client` regenerated and the web app building green ([ADR-0035](../adr/ADR-0035-one-product-version.md) §4). A core task adds no screens, but it may not leave the client lane red either.

---

## 4. Performance guidelines

| Rule | Reason |
|---|---|
| Every query starts with `tenant_id` in the index | The RLS predicate and selectivity |
| No `N+1`: `expand` resolves relations with batch queries (`IN`) | A kanban board loads hundreds of items |
| No `COUNT(*)` over large sets on the standard path | Estimate, or use a cursor |
| `statement_timeout` set per role (short for the API, longer for workers) | Protection against a runaway query |
| Cursors instead of offsets | Stable performance on deep pages |
| The write path: one transaction, no external calls | Keep latency and lock times small |
| External effects asynchronously through the outbox | Response time independent of third-party systems |
| Large operations as a job with progress | No request over 30 s |
| Targets | P95 read < 200 ms, P95 write < 300 ms at 10⁶ items per tenant |

---

## 5. Operating guidelines

The full concept: [observability-reliability.md](./observability-reliability.md)
(SLOs, metric and alert catalogue, resilience patterns, degradation matrix, runbooks).
The hard requirements in short:

| Topic | Requirement |
|---|---|
| Health | `/healthz` (the process only — **never** check dependencies), `/startupz`, `/readyz` (database plus mandatory dependencies, migration state), `GET /api/v1/meta/health` (deep self-diagnosis including configuration warnings) |
| Graceful shutdown | `SIGTERM` → no new requests, finish in-flight ones, release jobs; `terminationGracePeriodSeconds` ≥ the job timeout |
| Migrations | A dedicated job or init container, never during API startup; an advisory lock prevents parallel runs |
| Idempotent jobs | Every job must be runnable more than once (at-least-once semantics) |
| Backups | PITR as the standard, `pg_dump` as the self-hosting minimum, the media bucket separately; RPO ≤ 5 min, RTO ≤ 60 min; a restore drill with consistency and isolation checks is a **release criterion** (RT-9); a missing backup configuration produces a warning in `/meta/health` |
| Logs | Structured JSON, no user data content, always `tenant_id` and `request_id` |
| Metrics | RED per use case, queue depth, outbox lag, webhook success rate, rule runs, occurrence lag, database pool utilisation |
| Alerts | A complete symptom-based catalogue A-01…A-18 with a runbook per alert; an alert without a runbook does not ship |
| Resilience | A timeout on every call, a circuit breaker per external dependency, bulkheads between API and worker, load shedding, dead letter instead of endless retry |
| Degradation | The failure of an optional dependency terminates no process and never blocks the core write path; the affected feature is reported as a `degraded_feature` |
| Panics | Caught per request and per job; no bare goroutines (`SafeGo`); the metric's target value is permanently 0 |
| Operating material | Dashboards, Prometheus rules, and runbooks live under `deploy/observability/` in the repository |
| Container | Distroless, non-root, read-only root filesystem, no shell, multi-arch (amd64/arm64 — Raspberry Pi self-hosting) |
| Resources (starting values) | Self-hosting: 256 MB RAM / 0.25 vCPU; the `api` pod: 512 MB / 0.5 vCPU |

---

## 6. Security in the process

The full concept including the threat model and the gates: [security.md](./security.md),
decision [ADR-0015](../adr/ADR-0015-security-baseline.md).

* A threat model per bounded context (STRIDE short form) at the first design, and thereafter on any security-relevant change.
* Dependency updates automated (Dependabot), `govulncheck` in CI, a container scan in the release.
* Secrets never in the repository; secret scanning active (push rule).
* Security-relevant changes need a second reviewer.
* Responsible disclosure per `SECURITY.md`, advisories with CVSS and affected versions.
* Standard hardening: security headers, restrictively configurable CORS, rate limits from day one, Argon2id for passwords, tokens stored only hashed, uploads validated, outbound calls through `GuardedClient`.

---

## 7. Tooling

| Purpose | Tool |
|---|---|
| Build / task runner | `make` (+ `go tool`), reproducible builds |
| Go | The toolchain is pinned to a patched release in `go.mod`; an unpatched standard library is a `govulncheck` finding |
| Lint | `golangci-lint` (errcheck, govet, staticcheck, revive, depguard, gosec); every tool version pinned in the `Makefile` |
| Gate self-test | `make gate-selftest` - one deliberate violation per rule, each expected to fail the build |
| Code generation | `oapi-codegen` (API), `sqlc` (database), a custom generator for the MCP manifest |
| Migrations | `goose` |
| Tests | `testing`, `testcontainers-go`, `k6` |
| Container | Docker/Buildx, a distroless base |
| Deployment | Helm (template in `k8s/`), Compose for self-hosting |
| Observability | OpenTelemetry SDK, Prometheus, OTel Collector |
| Documentation | Markdown + Mermaid in the repository; arc42 as the structure |
