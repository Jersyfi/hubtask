# ADR-0016: Self-diagnosis, controlled degradation, and SLO-based operation

* **Status:** accepted
* **Date:** 2026-08-14
* **Concerns:** operations, reliability, observability
* **Related:** [ADR-0002](./ADR-0002-modular-monolith.md), [ADR-0008](./ADR-0008-jobs-and-scheduling.md), [ADR-0014](./ADR-0014-single-image-multi-role.md), [observability-reliability.md](../architecture/observability-reliability.md)

## Context

The application should "essentially never crash and always know what it is missing". It runs in two
very different environments: on a private individual's single machine with no operations team, and
at providers in Kubernetes with an on-call rota. It depends on several optional services (object
storage, SMTP, AI providers, optionally a search index and NATS) and performs background work whose
failure can stay invisible to users — a reminder that never fires produces no error message, it
simply does not happen.

The usual answer — "the process checks every dependency at startup and exits on a problem" — is
harmful here: in Kubernetes it produces crash loops across every pod as soon as a peripheral
dependency fails, and in self-hosting it produces a container that will not come back up after a
MinIO restart.

## Decision

1. **A separated health model:** `/healthz` checks only whether the process responds (never dependencies), `/readyz` checks the ability to serve traffic, `/startupz` checks initialisation, and `GET /api/v1/meta/health` supplies the deep self-diagnosis with dependency status, degradation states, backlogs, and **configuration warnings**.
2. **Controlled degradation is mandatory:** the failure of an optional dependency must not block the core write path and must not terminate a process. Affected features are explicitly reported as `degraded_features` with a reason and a timestamp.
3. **SLOs with an error budget** (SLO-1 … SLO-8) are part of the repository; from 50% budget consumption, stability work takes precedence over features.
4. **Resilience patterns are binding**, not optional: timeouts everywhere, a circuit breaker per external dependency, bulkheads between the interactive and the background path, load shedding, dead letter instead of endless retry, idempotency everywhere.
5. **No bare goroutines:** concurrency exclusively through `SafeGo` with panic recovery and a metric; `hubtask_panics_recovered_total` is an alert metric with a target value of 0.
6. **Observability is part of the Definition of Done** and is enforced by gate RT-12: every use case in the registry must produce a metric and a trace span, or the build fails.
7. **Operating material ships too**, not just code: dashboards, alert rules, and runbooks live under `deploy/observability/`; an alert without a runbook does not ship.

## Options considered

| Option | Assessment |
|---|---|
| **Chosen: self-diagnosis + degradation + SLOs** | Serves both operating profiles with identical code; a higher implementation effort in the adapters. |
| Fail fast at startup (check dependencies, otherwise exit) | Simple, but it produces crash loops and directly contradicts "never crash". Rejected. |
| Everything synchronous, no degradation states | A simpler codebase, but a slow webhook recipient or a disrupted object store brings the API to a standstill. Rejected. |
| An external service mesh for retries, timeouts, and breakers | It solves only the network level, knows nothing about business backlogs (outbox lag, reminder delay), and is unavailable in self-hosting. It remains possible as an optional addition in provider operation. |
| Logs only, analysed on demand | No early warning for silent failures (reminders that never fired, rules that failed). Rejected. |

## Consequences

**Positive**

* Failures of individual services become feature restrictions rather than outages.
* The operator sees what is missing without a third-party tool — including the private user without Prometheus (`/meta/health` with `warnings`).
* Root cause analysis across process and system boundaries, because the `trace_id` travels through the outbox and the job queue.
* Silent failures in background work become visible (dead letter metrics, reminder delay, rule deactivations).

**Negative / countermeasures**

* *More code in every adapter* (breakers, timeouts, status reporting). → One-off building blocks in `core/shared` and `infrastructure/resilience`; adapters compose them rather than reinventing them.
* *Degradation states multiply the test matrix.* → The fixed test series RT-1 … RT-12 with test containers instead of arbitrary ad-hoc tests.
* *Metrics can drive cardinality and cost.* → Hard label rules; `tenant_id` as a label only when explicitly enabled, and no object IDs.
* *An SLO process creates overhead for a young project.* → Before `1.0.0`, SLOs count as measurements without an on-call obligation; the error budget process takes effect only in commercial operation.
* *`/meta/health` is itself a source of information for an attacker.* → The endpoint is authenticated with an admin scope; `/healthz` and `/readyz` return only status codes without detail.
