# Milestone 0.1.0 — Walking Skeleton

The goal: a runnable, deployable, tested system with **exactly one** business use case, in which
every cross-cutting concern has been driven through once, completely. After that, every further use
case has a pattern to copy.

Every task is one pull request. The order is binding where dependencies exist.

Legend: **[L]** = best done locally with Claude Code (you see every step),
**[G]** = delegable through a GitHub issue.

---

## A-01 — Bring the toolchain and gates to life **[L]**

*Depends on: nothing. The first task.*

`make tools`, `make verify`, and all the `gate-*` targets must actually run through — several of
them are placeholders today. Set up `.golangci.yml` with the linters from the engineering guidelines
(errcheck, govet, staticcheck, revive, depguard, gosec, noctx); configure `depguard` so that it
knows the layer boundaries.

**Acceptance:** `make verify` green on a fresh clone; a deliberate violation of every configured
rule demonstrably fails the build; CI runs with no red jobs.

**Read:** `engineering-guidelines.md` §7, `ci-cd.md`, `project-structure.md` §Dependency rules

---

## A-02 — Complete the configuration, logging, and error model **[L]**

*Depends on: A-01*

Extend the existing skeleton (`core/port/environment`, `infrastructure/environment`) with the
complete configuration: the database pool, object storage, SMTP, rate limits, time zone, locale
default. Create the error model from `arc42.md` §8.11: typed domain errors, categories
(`VALIDATION`, `NOT_FOUND`, `CONFLICT`, `FORBIDDEN`, `RATE_LIMITED`, `INTERNAL`), and the mapping to
RFC 9457 responses with a stable `code`, a `request_id`, and message parameters.

**Acceptance:** a missing required secret prevents startup with a clear message; every error
category has a test; no display text in the backend; log output demonstrably contains no secrets
(a test).

**Read:** `api-guidelines.md`, `i18n-l10n.md`, `security.md` §T-18

---

## A-03 — The PostgreSQL connection with an enforced tenant boundary **[L]**

*Depends on: A-02. Security-critical — please read along on this one.*

`pgxpool`, the transaction wrapper as the **only** place with `SET LOCAL app.tenant_id`,
`UnitOfWork`, the `goose` migration `0001_init.sql` from `db/schema.sql`, and the roles
`hubtask_app` (without `BYPASSRLS`) and `hubtask_migrator`. A health probe for PostgreSQL that
replaces the registry.

**Acceptance:** the test "the app role has no `BYPASSRLS`"; the test "RLS is active on every tenant
table" through a catalogue query against the table list; access without a tenant context set
returns zero rows; the migration runs against an empty and against an existing database.

**Read:** `multi-tenancy.md`, ADR-0010, ADR-0003, `security.md` §6

---

## A-04 — The observability skeleton **[G]**

*Depends on: A-02*

OpenTelemetry (traces, metrics), the Prometheus endpoint on the ops port, `slog` with trace
correlation and redaction, wiring up the `SafeGo` panic metric, RED metrics per use case, and
`/api/v1/meta/health` with dependencies, degradation states, and configuration warnings.

**Acceptance:** `hubtask_panics_recovered_total` exists and stays at 0; `/meta/health` returns the
schema from `openapi.yaml`; `config.backup_not_configured` appears as a warning when no backup
target is configured; label cardinality checked (no object IDs).

**Read:** `observability-reliability.md` §3–5, ADR-0016

---

## A-05 — The resilience building blocks **[G]**

*Depends on: A-04*

`infrastructure/resilience`: timeout helpers, retry with backoff and jitter, a circuit breaker, a
bulkhead, a load shedder. `infrastructure/httpclient.GuardedClient` with SSRF protection (resolve
before connecting, a block list, rebinding protection, a redirect check, a size limit).

**Acceptance:** the SSRF test suite against metadata addresses, private networks, DNS rebinding, and
redirect chains (gate SG-6); the breaker demonstrably opens and closes; test RT-1 (the failure of an
optional dependency does not block the write path).

**Read:** `security.md` §T-07, `observability-reliability.md` §6, ADR-0016

---

## A-06 — The API skeleton from the specification **[L]**

*Depends on: A-02, A-03*

Set up `oapi-codegen`, generate the router from `api/openapi.yaml`, and build the middleware chain:
request ID, auth (PAT only at first), tenant context, locale, rate limit, `Idempotency-Key`, panic
recovery, security headers, CORS. Implement `/meta/capabilities`.

**Acceptance:** `make generate` produces no diff; a contract test validates responses against the
specification; `/meta/capabilities` returns item types and capability profiles from the database,
not from constants.

**Read:** `api-guidelines.md`, ADR-0004, ADR-0005

---

## A-07 — Drive the reference use case `CreateContainer` through **[L]**

*Depends on: A-03, A-04, A-06. The most important piece of work in this milestone.*

A single use case, but complete: the `Container` domain model with invariants I-C1…I-C3, the
repository, the `AuthorizationService`, the use case registry with registration for REST **and** MCP
**and** as an automation action, the domain event
`de.hubtask.work.container.created.v1` through the outbox, an audit entry, a metric, a
trace span, a change log entry for the sync, and message codes.

**Acceptance:** the parity test (REST/MCP/automation) green; a cross-tenant negative test per
repository method; the event schema under `api/events/`; a change log entry with an HLC; the
complete Definition of Done ticked off.

**Read:** `domain-model.md` §3.3 and §5, `automation.md`, `ai-first.md`, `offline-sync.md` §4,
`audit.md` §2

---

## A-08 — Job queue, outbox, leader election **[G]**

*Depends on: A-03, A-05*

The `SKIP LOCKED` queue, the outbox dispatcher, the advisory lock leader for the `scheduler` role,
dead letter with context, and resumption after process death.

**Acceptance:** test RT-3 (`SIGKILL` mid-job → takes effect exactly once); test RT-4 (duplicate
delivery without duplicate effect); outbox lag as a metric; two scheduler instances with only one
active.

**Read:** ADR-0008, ADR-0007, `observability-reliability.md` §6

---

## A-09 — The deployment skeleton **[G]**

*Depends on: A-07*

Helm templates alongside the existing `values.yaml`: one deployment per role, a service, the
migration job as a pre-upgrade hook, a PDB, optionally an HPA, a ServiceMonitor, a NetworkPolicy.
Check the Compose file against the real image.

**Acceptance:** `helm template` and `helm lint` without errors; `docker compose up` starts from the
published image and reaches a green `/readyz` in under five minutes; a rolling update without `5xx`
under load (test RT-8, manually at first).

**Read:** `deployment.md`, ADR-0023, ADR-0014

---

## A-10 — Finish the repository hygiene **[G]**

*Can run in parallel.*

The licence construct is in place: `LICENSE` (BSL 1.1 with the licensor filled in), `LICENSE-APACHE`,
`NOTICE`, `TRADEMARK.md`, `CLA.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, and the
SPDX headers in the Go files. What remains: have `THIRD-PARTY-LICENSES.md` generated
(`go-licenses`) and wire the licence check into CI (block GPL/AGPL dependencies), implement
`make gate-docs` (`tools/checkdocs`), and extend the data catalogue with the fields actually
created.

**Acceptance:** `THIRD-PARTY-LICENSES.md` generated as part of the release; the CI licence gate
rejects a GPL dependency; `make gate-docs` green.

**Read:** `licensing-editions.md`, ADR-0013, `data-protection.md`

---

## The order at a glance

```
A-01 ─┬─ A-02 ─┬─ A-03 ─┬─ A-06 ─┬─ A-07 ─┬─ A-09
      │        │        │        │        │
      │        ├─ A-04 ─┴─ A-05 ─┴─ A-08 ─┘
      │        │
      └────────┴─ A-10 (any time)
```

**Definition of Done for the milestone:** a fresh clone runs `make verify` green, the Compose setup
starts from the published image, `CreateContainer` is reachable through REST, MCP, and automation,
and every CI gate is green — including the cross-tenant suite and the architecture rules.
