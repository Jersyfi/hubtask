# Engineering Guidelines

How work is tested, when it may start and when it is finished. Binding for all contributions;
complements [arc42.md](./arc42.md) chapters 10 and 11.

---

## 1. Test strategy

| Level | Scope | Tooling | Gate | Runtime budget |
|---|---|---|---|---|
| **Domain** | Invariants, state transitions, capabilities, hierarchy, recurrence | Table tests, no mocks, no database | `make gate-unit` (coverage ≥ 85 % per package) | < 10 s in total |
| **Application** | Use cases, permissions, idempotency, events | Fakes of the ports beside the tests | `make gate-unit` (coverage ≥ 75 % per package) | < 30 s |
| **Infrastructure** | Repositories, RLS, queue, outbox, migrations | Testcontainers with a real PostgreSQL — a mock of RLS would only test the mock | `make gate-integration` | < 5 min |
| **Contract** | REST, events and MCP tools against their schemas | `test/contract` | `make gate-contract` | < 1 min |
| **End-to-end** | The core paths over HTTP against the whole process | `scripts/hubctl-e2e.sh` on the Compose stack; Playwright for the web app ([ADR-0048](../adr/ADR-0048-browser-job-driver.md)) | `make gate-e2e`, `make gate-compose` | < 5 min |
| **Load** | Query DSL at scale, automation storm, overload | `test/load` against the real binary, two tiers ([observability-reliability.md](./observability-reliability.md) §13.2) | `make gate-load` | Nightly / before a release |
| **Architecture** | Layers, use case parity, the `go` ban, authorisation, message codes | `depguard`, `test/architecture` ([project-structure.md](./project-structure.md) §2) | `make gate-architecture` | < 10 s |

**Mandatory test cases with a history of going wrong** (golden files where the output is data):
DST both ways in several zones, 29 February in RRULE, `FREQ=MONTHLY;BYMONTHDAY=31`, a time zone
change with reminders set, Unicode titles (emoji, RTL, combining) in length checks and search, a
cross-tenant negative test per repository method, a subtree moved across collections, an automation
loop to the abort depth, partial failure in a bulk operation.

No test depends on randomness or the system clock: `Clock`, `IDGenerator` and `RandomSource` are
injected ([arc42.md](./arc42.md) §8.13).

---

## 2. Definition of Ready (the story may be implemented)

1. The bounded context and aggregate are named.
2. The task's `**Use cases:**` line names the use cases ([`docs/usecases/`](../usecases/README.md)) and the checks the work makes true; its operations are named from [domain-model.md](./domain-model.md) §5.
3. API impact settled (operations, fields, error codes); a breaking change needs an ADR.
4. Domain events named, with their compatibility.
5. Permissions: which role at which scope may do this.
6. Message codes named; no display text in the backend.
7. The `tenant_id` path and RLS impact checked.
8. Provided for as an automation action and/or trigger.
9. Migration and expand/contract steps sketched.
10. Acceptance criteria testable, error cases included.
11. **Security assessment**: new attack surface named; threats (T-xx) in [security.md](./security.md) checked or added.
12. **Failure behaviour**: the dependency touched and what degrades when it fails ([observability-reliability.md](./observability-reliability.md) §7).
13. **Data protection**: new personal data in the data catalogue with purpose, retention, deletion path ([data-protection.md](./data-protection.md)).
14. **Audit obligation**: a security- or compliance-relevant operation is in the `AuditableAction` registry.
15. **Sync impact**: change log entries produced, each field's merge rule ([offline-sync.md](./offline-sync.md) §4).
16. **Client availability**: the area of the client capability matrix; a restriction beyond [arc42.md](./arc42.md) §8.19 is recorded there with its reason and an ADR.
17. **No other product's name in the implementation** ([ADR-0061](../adr/ADR-0061-page-anatomy-and-the-shell.md) decision 6): not in code, comments, identifiers, commits, pull request or issue text, the catalogue, the UI, the website, the workbench or a specification. An ADR may name where a pattern is proven in one sentence of context; a dependency is named where its licence requires; an import format carries its name.

---

## 3. Definition of Done

The pull request template carries the checkboxes; this section says what each means. An item that
does not apply is marked `n/a`, never deleted.

### 3.1 Tests green, the local check passed
The relevant levels of §1 and the coverage thresholds hold; `make verify-pr` is green for the pushed
`HEAD` before the pull request leaves draft ([ADR-0079](../adr/ADR-0079-a-draft-is-checked-locally.md)).

### 3.2 The contract first, generation clean
An API change starts in `api/openapi.yaml`; `make generate` leaves no diff on a second run;
generated code is never edited by hand ([ADR-0004](../adr/ADR-0004-api-first-openapi.md),
[project-structure.md](./project-structure.md) §6).

### 3.3 Registered in all three channels
A descriptor, an entry in `core/application/catalogue/Catalogue.go`, wiring in `cmd/server` — so a
REST operation, an MCP tool and an automation action; the parity gate checks it
([domain-model.md](./domain-model.md) §5).

### 3.4 Event schemas
A new or changed event has its JSON schema under `api/events/`, compatible as
[domain-model.md](./domain-model.md) §4 says.

### 3.5 A safe migration
A schema change is a new migration, safe for a rolling update (expand/contract) and tested against
the previous state ([versioning-release.md](./versioning-release.md) §4); `db/schema.sql` changes
with it.

### 3.6 Message codes
Every message code emitted is in `locales/en.json`; the backend sends codes and parameters, never a
sentence ([i18n-l10n.md](./i18n-l10n.md)).

### 3.7 Permissions and negative tests
The permission is checked in the application layer; every new repository method has a cross-tenant
negative test (gate SG-3).

### 3.8 Observability
Every use case has a metric **and** a trace span (gate RT-12), classified errors, and logs without
user content or secrets ([observability-reliability.md](./observability-reliability.md)).

### 3.9 Resilience
Timeouts or deadlines on every call, `SafeGo` only, external effects through the outbox or a job,
retried operations idempotent, the touched dependency's failure tested.

### 3.10 Security
Authorisation in the application layer, outbound calls through `GuardedClient`, the affected SG-x
gates green ([security.md](./security.md) §13).

### 3.11 Audit and data protection
Auditable actions registered and tested (SG-13); new personal data in
[the data catalogue](../privacy/data-catalog.md) with a deletion path; no user content in audit, logs
or metrics.

### 3.12 Retention, sync and the archive
A new data kind is in the retention catalogue; every new field has a merge rule
([offline-sync.md](./offline-sync.md) §4); the archive format and import follow a model change
(BK-4).

### 3.13 Documentation
The subject document holding the rule changes in the same pull request; an architectural decision
gets an ADR; the changelog comes from the commit titles.

### 3.14 Commit titles
Conventional Commits; a breaking change is marked
([versioning-release.md](./versioning-release.md) §3).

### 3.15 Client impact
An `api/openapi.yaml` change regenerates `packages/api-client` and keeps the web app green in the same
pull request ([ADR-0035](../adr/ADR-0035-one-product-version.md)); a core task adds no screens.

### 3.16 Use cases met
Every check in the task's `**Use cases:**` line holds with a test or walk as evidence, recorded in the
use case's `state:`, `checked_by:` and *Today* and reported in the pull request; the checklist in
[`docs/usecases/README.md`](../usecases/README.md#checking-work-against-its-use-cases) finds nothing
open.

### 3.17 Design values only from tokens
No colour, spacing, radius or duration value outside
`packages/design-system/tokens/tokens.json`, and `make tokens` produces no diff
([ADR-0029](../adr/ADR-0029-design-system-tokens.md)).

### 3.18 The frontend boundary
`core/` knows nothing about a frontend, and no `.go` file is committed under `apps/` or `packages/`
([project-structure.md](./project-structure.md) §2.1).

### 3.19 Reviewed against what no gate checks
The author reviews against every [AGENTS.md](../../AGENTS.md) rule marked `[partial]`, `[unchecked]`
or `[owner]` and names the findings here: fixed in a commit, or a `finding` issue.

---

## 4. Performance guidelines

| Rule | Reason |
|---|---|
| Every query starts with `tenant_id` in the index | The RLS predicate and selectivity |
| No `N+1`: batch queries (`IN`) | A board loads hundreds of items |
| A total only with `?count=exact` | A second scan nobody asked for |
| `statement_timeout` per role (short for the API, longer for workers) | A runaway query |
| Cursors instead of offsets | Stable performance on deep pages |
| The write path: one transaction, no external calls | Short latency and locks |
| External effects through the outbox | Independent of third parties |
| Large operations as a job with progress | No request over 30 s |
| Targets | P95 read < 200 ms, P95 write < 300 ms at 10⁶ items per tenant |

---

## 5. Operating guidelines

The concept is [observability-reliability.md](./observability-reliability.md) and
[deployment.md](./deployment.md). The rules a change most often touches:

* **Health:** `/healthz` checks the process only, **never** a dependency; `/readyz` checks the
  database, the mandatory dependencies and the migration state; `GET /api/v1/meta/health` is the
  deep self-diagnosis.
* **Migrations** run as a dedicated job or init container, never during API startup, under an
  advisory lock against parallel runs.
* **Jobs** can run more than once (at-least-once).
* **Degradation:** an optional dependency's failure ends no process and never blocks the core write
  path; the feature appears in `degraded_features`.
* **Panics** are caught per request and per job; no bare goroutines (`SafeGo`).
* **Alerts:** none without a runbook; both live under `deploy/observability/`.

---

## 6. Security in the process

The concept, threat model and gates are [security.md](./security.md)
([ADR-0015](../adr/ADR-0015-security-baseline.md)). In the process:

* A new bounded context gets a short STRIDE analysis at design time; a security-relevant change
  updates the threat model (DoR item 11).
* A suppressed lint finding carries its reason in the code (`nolintlint`); softening a gate needs an
  ADR.
* Responsible disclosure follows `SECURITY.md`.

---

## 7. Tooling

* `make` is the task runner: every gate is a target, every tool version pinned in the `Makefile`.
* Go is pinned to a patched release in `go.mod`; an unpatched standard library is a `govulncheck`
  finding.
* Lint: `golangci-lint` (errcheck, govet, staticcheck, revive, depguard, gosec, noctx,
  contextcheck, errorlint and others — `.golangci.yml`).
* `make gate-selftest` plants one deliberate violation per rule, each expected to fail.
* Code generation: `oapi-codegen` (server and Go SDK), `sqlc`, `tools/openapijson`, `tools/sdkgen`,
  `tools/eventmatrix`, `tools/deprecations`; migrations: `goose`.
* Tests: `testing`, `testcontainers-go`, Playwright (the web app).
* Delivery: Docker/Buildx on a distroless base; Helm (the chart in `k8s/`), Compose for
  self-hosting; OpenTelemetry SDK, Prometheus, OTel Collector.
* Documentation: Markdown + Mermaid in the repository, checked by `make gate-docs`.
