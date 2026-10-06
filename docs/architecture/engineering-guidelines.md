# Engineering Guidelines

How work is tested, when it may start and when it is finished. Binding for all contributions;
complements [arc42.md](./arc42.md) chapters 10 and 11.

---

## 1. Test strategy

| Level | Scope | Tooling | Gate | Runtime budget |
|---|---|---|---|---|
| **Domain** | Invariants, state transitions, capability rules, hierarchy, recurrence, assignment strategies | Pure table tests, no mocks, no database | `make gate-unit` (coverage ≥ 85 % per package) | < 10 s in total |
| **Application** | Use cases with in-memory fakes of the ports; permissions, idempotency, event emission | Standard `testing`, fakes beside the tests | `make gate-unit` (coverage ≥ 75 % per package) | < 30 s |
| **Infrastructure** | Repositories, RLS, queue, outbox, migrations | Testcontainers with a real PostgreSQL — a mock of RLS would only test the mock | `make gate-integration` | < 5 min |
| **Contract** | REST responses against `openapi.yaml`, events against JSON Schema, MCP tool schemas | Schema validation in the test (`test/contract`) | `make gate-contract` | < 1 min |
| **End-to-end** | The core paths over HTTP against the complete process | `scripts/hubctl-e2e.sh` against the Compose stack; Playwright for the web app ([ADR-0048](../adr/ADR-0048-browser-job-driver.md)) | `make gate-e2e`, `make gate-compose` | < 5 min |
| **Load** | The query DSL over a large dataset, automation storm, overload | `go test -tags load ./test/load/` against the real binary; two tiers, see [observability-reliability.md](./observability-reliability.md) §13.2 | `make gate-load` | Nightly / before a release |
| **Architecture** | Import and layer rules, use case parity, the `go` ban, authorisation, message codes | `depguard` in `golangci-lint` plus `test/architecture` ([project-structure.md](./project-structure.md) §2) | `make gate-architecture` | < 10 s |

**Mandatory test cases with a history of going wrong** (golden files where the output is data):
DST transitions in both directions for several time zones, leap year / 29 February in RRULE,
`FREQ=MONTHLY;BYMONTHDAY=31`, an account changing time zone with existing reminders, Unicode
titles (emoji, RTL, combining characters) in length checks and search, cross-tenant negative tests
for every repository method, moving a subtree across collection boundaries, an automation loop
running to the abort depth, partial failure in a bulk operation.

No test depends on randomness or the system clock: `Clock`, `IDGenerator` and `RandomSource` are
injected ([arc42.md](./arc42.md) §8.13).

---

## 2. Definition of Ready (the story may be implemented)

1. The bounded context and aggregate are named.
2. The use cases the work serves are named in the task's `**Use cases:**` line, from [`docs/usecases/`](../usecases/README.md), with the checks it makes true; the operations it adds or changes are named from the catalogue in [domain-model.md](./domain-model.md) §5.
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
16. **Client availability** named: which area of the client capability matrix the feature belongs to (end-user, profile configuration, administration); a restriction beyond the matrix in [arc42.md](./arc42.md) §8.19 needs its justification recorded there, with an ADR.
17. **No other product's name in the implementation** ([ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md) decision 6): not in code, comments, identifiers, commit titles or bodies, pull request or issue text, the catalogue, the UI, the website, the workbench or a specification. An ADR may carry one sentence of context naming where a pattern is proven; a dependency is named where its licence requires; an import format carries the format's name.

---

## 3. Definition of Done

The pull request template carries the checkboxes; this section says what each item means. An item
that does not apply is marked `n/a` in the template, never deleted.

### 3.1 Tests green, the local check passed
Every relevant level of §1 is green and the coverage thresholds hold. `make verify-pr` is green for
the pushed `HEAD` before the pull request leaves draft: CI runs only on a ready pull request, so the
local run is the check a draft gets ([ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md)).

### 3.2 The contract first, generation clean
A change to the API changes `api/openapi.yaml` first; `make generate` then runs and produces no diff
on a second run. Generated code is never edited by hand
([ADR-0004](../adr/ADR-0004-api-first-openapi.md), [project-structure.md](./project-structure.md) §6).

### 3.3 Registered in all three channels
The use case has a descriptor, is listed in `core/application/catalogue/Catalogue.go` and wired in
`cmd/server`, and is therefore a REST operation, an MCP tool and an automation action. The parity
gate in `make gate-architecture` fails otherwise ([domain-model.md](./domain-model.md) §5).

### 3.4 Event schemas
A new or changed event has its JSON schema under `api/events/`, and its compatibility follows
[domain-model.md](./domain-model.md) §4.

### 3.5 A safe migration
A schema change is a new migration — never an edit of an existing one — safe for a rolling update
(expand/contract), and tested against the previous state
([versioning-release.md](./versioning-release.md) §4). `db/schema.sql` is updated with it.

### 3.6 Message codes
Every message code the change emits is in `locales/en.json`; the backend sends codes and
parameters, never a sentence ([i18n-l10n.md](./i18n-l10n.md)).

### 3.7 Permissions and negative tests
The permission is checked in the application layer, and every new repository method has a
cross-tenant negative test — gate SG-3 reconciles methods against tests.

### 3.8 Observability
Every use case has a metric **and** a trace span (gate RT-12), its errors are classified, and its
logs carry no user content and no secret
([observability-reliability.md](./observability-reliability.md)).

### 3.9 Resilience
Every call has a timeout or a context deadline; concurrency goes only through `SafeGo`; external
effects go through the outbox or a job; the operation is idempotent where it is retried; and the
failure of the dependency it touches has been tested.

### 3.10 Security
Authorisation is in the application layer, outbound calls go through `GuardedClient`, and the
affected SG-x gates are green ([security.md](./security.md) §13).

### 3.11 Audit and data protection
An auditable action is in the `AuditableAction` registry and tested (gate SG-13); a new personal
data field is in [the data catalogue](../privacy/data-catalog.md) with a deletion path; no user
content reaches the audit trail, the logs or the metrics.

### 3.12 Retention, sync and the archive
A new data kind is in the retention catalogue; every new field has a merge rule
([offline-sync.md](./offline-sync.md) §4); the archive format and the import path follow a model
change (BK-4).

### 3.13 Documentation
The subject document that holds the rule is updated in the same pull request. An architectural
decision gets a new ADR, and the changelog comes from the commit titles.

### 3.14 Commit titles
Conventional Commits; a breaking change is marked
([versioning-release.md](./versioning-release.md) §3).

### 3.15 Client impact
A change to `api/openapi.yaml` carries its client fix in the same pull request:
`packages/api-client` regenerated and the web app building green
([ADR-0035](../adr/ADR-0035-one-product-version.md)). A core task adds no screens, but it may not
leave the client lane red.

### 3.16 Use cases met
Every check of every use case the task names holds, with a test or a walk as evidence; the use
case's `state:`, `checked_by:` and *Today* say so; `/usecase-check` over the branch finds nothing
open; and the pull request's *Use cases* section reports each check
([`docs/usecases/README.md`](../usecases/README.md)).

### 3.17 Design values only from tokens
No colour, spacing, radius or duration value is written outside
`packages/design-system/tokens/tokens.json`, and `make tokens` produces no diff
([ADR-0029](../adr/ADR-0029-design-system-tokens.md)).

### 3.18 The frontend boundary
`core/` learned nothing about a frontend, and no `.go` file is committed under `apps/` or
`packages/` ([project-structure.md](./project-structure.md) §2.1).

---

## 4. Performance guidelines

| Rule | Reason |
|---|---|
| Every query starts with `tenant_id` in the index | The RLS predicate and selectivity |
| No `N+1`: relations are resolved with batch queries (`IN`) | A kanban board loads hundreds of items |
| No `COUNT(*)` over large sets on the standard path; a total only with `?count=exact` | A total is a second scan nobody asked for |
| `statement_timeout` set per role (short for the API, longer for workers) | Protection against a runaway query |
| Cursors instead of offsets | Stable performance on deep pages |
| The write path: one transaction, no external calls | Keep latency and lock times small |
| External effects asynchronously through the outbox | Response time independent of third-party systems |
| Large or unbounded operations as a job with progress | No request over 30 s |
| Targets | P95 read < 200 ms, P95 write < 300 ms at 10⁶ items per tenant |

---

## 5. Operating guidelines

The full concept is [observability-reliability.md](./observability-reliability.md) (SLOs, metrics,
alerts, resilience, degradation, runbooks) and [deployment.md](./deployment.md). The rules a change
most often touches:

| Topic | Rule |
|---|---|
| Health | `/healthz` checks the process only and **never** a dependency; `/readyz` checks the database, the mandatory dependencies and the migration state; `GET /api/v1/meta/health` is the deep self-diagnosis |
| Migrations | A dedicated job or init container, never during API startup; an advisory lock prevents parallel runs |
| Jobs | Every job can run more than once (at-least-once) |
| Degradation | The failure of an optional dependency terminates no process and never blocks the core write path; the feature is reported in `degraded_features` |
| Panics | Caught per request and per job; no bare goroutines (`SafeGo`) |
| Alerts | An alert without a runbook does not ship; both live under `deploy/observability/` |

---

## 6. Security in the process

The full concept, with the threat model and the gates, is [security.md](./security.md)
([ADR-0015](../adr/ADR-0015-security-baseline.md)). In the process:

* A new bounded context gets a short STRIDE analysis at design time, and a security-relevant change
  updates the threat model (DoR item 11).
* A suppressed lint finding carries its reason in the code (`nolintlint` requires it); softening a
  gate needs an ADR.
* Responsible disclosure follows `SECURITY.md`.

---

## 7. Tooling

| Purpose | Tool |
|---|---|
| Build / task runner | `make`; every gate is a target, every tool version pinned in the `Makefile` |
| Go | The toolchain is pinned to a patched release in `go.mod`; an unpatched standard library is a `govulncheck` finding |
| Lint | `golangci-lint` (errcheck, govet, staticcheck, revive, depguard, gosec, noctx, contextcheck, errorlint and others — `.golangci.yml`) |
| Gate self-test | `make gate-selftest` — one deliberate violation per rule, each expected to fail the build |
| Code generation | `oapi-codegen` (server and Go SDK), `sqlc` (database), `tools/openapijson`, `tools/sdkgen`, `tools/eventmatrix`, `tools/deprecations` |
| Migrations | `goose` |
| Tests | `testing`, `testcontainers-go`, Playwright (the web app) |
| Container | Docker/Buildx, a distroless base |
| Deployment | Helm (the chart in `k8s/`), Compose for self-hosting |
| Observability | OpenTelemetry SDK, Prometheus, OTel Collector |
| Documentation | Markdown + Mermaid in the repository, checked by `make gate-docs` |
