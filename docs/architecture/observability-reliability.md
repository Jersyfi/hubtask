# Observability and Reliability

The goal: the system runs stably, degrades in a controlled way instead of crashing, loses no data,
and **always knows for itself what it is missing**. The decision is
[ADR-0016](../adr/ADR-0016-observability-reliability.md); jobs and scheduling are
[ADR-0008](../adr/ADR-0008-jobs-and-scheduling.md) (§15).

---

## 1. Principles

| Principle | Meaning |
|---|---|
| **Self-diagnosis before alerting** | The process knows the state of every dependency and makes it machine-readable (`/readyz`, `/meta/health`). The operator has to guess nothing. |
| **Degrade rather than die** | The failure of an optional dependency (S3, SMTP, AI, NATS) reduces functionality. It terminates no process and blocks no write path. |
| **No silent failure** | Every discarded operation produces a metric plus a log plus, where it matters to the business, a visible state on the object. "Failed and nobody knows" is a bug. |
| **Alert on symptoms** | Alerts hang off user impact (an SLO violation, a backlog), not off CPU utilisation. |
| **Restart is the normal case** | Every process may die at any time. State lives in PostgreSQL; jobs are idempotent and at-least-once. |
| **Observability is part of the Definition of Done** | A use case without a metric, a trace span and error classification is not finished (RT-12, §12). |

---

## 2. Service level objectives

Reference values for provider operation. In self-hosting the same metrics apply, without the error
budget process.

| SLO | Indicator | Target | Window |
|---|---|---|---|
| SLO-1 API availability | The share of requests without a `5xx` and without a timeout | ≥ 99.9% | 30 days rolling |
| SLO-2 Read latency | P95 `GET`/`:query` | < 200 ms | 30 days |
| SLO-3 Write latency | P95 mutating operations | < 300 ms | 30 days |
| SLO-4 Event delivery | Outbox lag P99 | < 30 s | 7 days |
| SLO-5 Reminder punctuality | The share of reminders within 60 s of their target time | ≥ 99% | 30 days |
| SLO-6 Webhook delivery | The share delivered (after retries, excluding 4xx recipient errors) | ≥ 99.5% | 7 days |
| SLO-7 Automation | The share of rule runs without an internal error | ≥ 99.5% | 7 days |
| SLO-8 Data loss | Confirmed writes that get lost | **0** | Always |
| RPO / RTO | Data loss and recovery after a total outage | RPO ≤ 5 min (PITR), RTO ≤ 60 min | Per incident |

**Error budget rule:** once 50% of an SLO's budget is consumed, stability work takes precedence over
new features until it recovers.

The RPO and RTO are targets; what a restore drill measures stays internal (§13.2).

---

## 3. Signals

### 3.1 Logs

Structured JSON through `log/slog`. Mandatory fields: `ts`, `level`, `msg`, `service`, `role`,
`version`, `component`, `request_id`, `trace_id`, `span_id`, `tenant_id`, `actor_type`, `use_case`,
`error_code`.

* **`component` is not `role`.** The role is the process, and one process may serve several
  (`role=api,worker`). The component is the loop: `rest`, `worker.runner`, `worker.scheduler`,
  `worker.job_listener`, `api.change_listener`, `restore-drill`. Both are set where a unit of work
  begins, through the context seam the request ID travels in (`core/shared/correlation`), not at
  each call site.
* **`error_code` is a stable code, never a sentence**, so a query finds every instance of one
  failure.
* **No user content** (titles, notes, comments, attachment names), no tokens, no email addresses in
  clear text (hashed, or the account ID).
* Levels: `ERROR` only for states that require human action, otherwise `WARN`. Expected business
  errors (validation, `404`) are `INFO`.

### 3.2 Metrics

OpenTelemetry → a Prometheus endpoint on the operations port (9090, never public). **Label
cardinality is bounded:** a label is a small closed set written by hand — an outcome, a reason, a
kind, a class — never an identifier (item, user, rule, object, recipient, key), an endpoint or host
a tenant configured, a message subject, or a raw path a client chose. `tenant_id` appears only if
the operator enables it (`HUBTASK_METRICS_TENANT_LABEL=true`), because with many tenants it would
explode the cardinality.

### 3.3 Traces

OpenTelemetry, exported over OTLP/HTTP when `HUBTASK_TRACING_ENABLED` is set
([deployment.md](./deployment.md) §6.1). The W3C `traceparent` is adopted from incoming requests and
passed on to outbound calls, including across the outbox and the job queue (the `trace_id` is
persisted in the event or job), so *HTTP request → event → automation rule → webhook* is traceable
end to end. Sampling: 100% of errors, 100% of slow requests (> 1 s), otherwise
`HUBTASK_TRACING_SAMPLE_RATIO` (5% by default). An upstream decision to sample is honoured.

### 3.4 Business events

`activity_entry` and `rule_run` are observability visible to users: why was this task moved, which
rule fired and what did it do. That view is part of the product.

---

## 4. Metric catalogue (minimum scope)

The catalogue is `infrastructure/observability/Metrics.go`: every `hubtask_*` series with its type,
labels and purpose. The permitted label set is `allowedLabels` in `Metrics_test.go`, where adding a
label is a deliberate cardinality decision. What must hold for it:

* Every signal the SLOs (§2), alerts (§10) and dashboards (§11) read exists as a series: RED per
  route and per use case (`hubtask_usecase_total`, §4.1), the backlogs, the dependency and breaker
  states (`hubtask_dependency_up` is self-diagnosis as a time series), the build and migration
  version.
* **`hubtask_panics_recovered_total` must be permanently 0**, and dead-lettered jobs are always
  visible.
* A counter an alert reads is written even when its value is zero: a counter never written has no
  series, and an alert on it reads "no data" and is believed.
* **Not built:** `hubtask_db_query_duration_seconds` (by sqlc query name) and
  `hubtask_db_errors_total` (by `timeout`/`serialization`/`connection`); the outbox backlog is a
  field of `/meta/health`, not a series, and A-05 alerts on the lag.

### 4.1 The `result` label

`result` is `ok`, or **the error category of the domain error model in lower case** — one value
per category, never a summary of several: `validation`, `not_found`, `conflict`, `forbidden`,
`unauthenticated`, `gone`, `rate_limited`, `unavailable`, `internal`. The set is defined by
`core/domain/model/shared.Category`; the code derives the label rather than translating it.

A throttled request counted as `internal` would report a defect that did not happen. Coarser views
are a query, not a label:

```promql
# Our fault - the error budget of SLO-1
sum(rate(hubtask_usecase_total{result=~"internal|unavailable"}[5m])) by (use_case)
```

A label written coarsely cannot be refined afterwards; a closed set of ten values is bounded
cardinality.

---

## 5. The health model

Four levels, deliberately kept separate:

| Endpoint | Semantics | Who uses it |
|---|---|---|
| `/healthz` | The process is alive. **Checks no dependency** — otherwise a database outage kills every pod at once | Liveness probe |
| `/startupz` | Initialisation is complete | Startup probe |
| `/readyz` | The process can serve traffic: every mandatory dependency answers (PostgreSQL is the only one) and it is not shutting down. Answers a status code and a short reason only | Readiness probe, load balancer |
| `GET /api/v1/meta/health` (authenticated) | **Deep self-diagnosis**: per dependency the status, latency, last error and breaker state; per feature the degradation state; backlogs (outbox, queue, webhook retries); configuration warnings. **One route, two answers:** a credential holding `admin:tenants` reads all of it; anybody else needs `ops:read` and a workspace administrator's permission, and reads `status`, `version` and `degraded_features` only. The same full report is served unauthenticated at `/meta/health` on the operations listener, which answers `503` when the status is `down` | Operators, support, a status page, the client's health banner |

Neither `/startupz` nor `/readyz` compares the pod's schema with the database's yet
([deployment.md](./deployment.md) §8, D-5).

The report's shape is the `/meta/health` schema in `api/openapi.yaml`. Its `warnings` are the
direct expression of "always know what it is missing": every gap is a code with a severity, never
free text; the codes are `Warnings` in `infrastructure/environment/EnvConfig.go`.

---

## 6. Resilience patterns

| Pattern | Binding implementation |
|---|---|
| **Timeouts everywhere** | No `http.Client`, no database query, no job without a deadline (`noctx`, `contextcheck`). The client too: `FetchTransport` in the sync engine refuses a transfer without a positive timeout |
| **Context propagation** | `ctx` is passed through; a client abort ends the work (except for an already committed transaction) |
| **Retry with backoff + jitter** | Only for idempotent operations; exponential, capped, with a maximum attempt count; never in the synchronous request path against third-party systems |
| **Circuit breaker** | Per external dependency (object storage, SMTP, AI per endpoint, each webhook target). Open → an immediate error instead of a blocked call; half-open with a probe. The state is a metric and is in `/meta/health` |
| **Bulkheads** | Separate connection and worker pools for API, worker and automation, so a runaway rule cannot starve the interactive path; under Kubernetes, separate deployments on top |
| **Load shedding** | Above `HUBTASK_LOAD_SHED_INFLIGHT` requests in flight (per role), new *deferrable* requests (`rest.DeferrableRoutes`: bulk, export, search, the query shapes) are refused with `503` + `Retry-After` before latency tips over for everyone. A parked stream is not load (`rest.LongLivedRoutes`) |
| **Rate limits** | Internally too: automation rules have throttles per rule and per tenant |
| **A queue instead of synchrony** | Everything external goes through the outbox or jobs. A hanging webhook recipient cannot delay an API response |
| **Idempotency** | `Idempotency-Key` on the outside, `job.dedupe_key` on the inside, `delivery_id` for webhooks. At-least-once plus idempotency = effectively exactly-once |
| **Poison pill protection** | After *n* failed attempts → dead letter with full context, a metric and admin visibility; the queue stays clear |
| **Optimistic locking** | A `version` per aggregate; a `409` with a machine-readable conflict instead of data loss through last-write-wins |
| **Panic recovery** | Middleware per request, a wrapper per job, and a **ban on bare goroutines**: concurrency only through `SafeGo(ctx, name, fn)` with recover plus a metric, enforced by an architecture test |
| **Memory and resource protection** | `GOMEMLIMIT` below the container's memory limit; streaming instead of full buffering for uploads and exports; capped result sets. OOM kills are an architecture defect. Neither the image nor the chart sets `GOMEMLIMIT` yet (§14, O-5) |
| **Clock robustness** | The scheduler catches up (bounded catch-up after an outage) and tolerates time jumps; no assumption that "the tick arrived on time" |

---

## 7. Controlled degradation

| Failed dependency | Behaviour | What the user sees |
|---|---|---|
| PostgreSQL | `/readyz` red, no traffic accepted, reconnection with backoff; **no** process exit | `503` with a message code, `Retry-After` |
| Object storage (S3-compatible) | Core features normal; upload/download disabled, `degraded_features` set | Attachments temporarily unavailable, tasks work |
| SMTP / push | Notifications stay in the queue and are caught up; no loss | The reminder arrives late, with an in-app notice |
| `LISTEN/NOTIFY` (the stream's wake-up) | Streams fall back to their idle poll interval; no record is lost or reordered | Changes arrive within seconds instead of immediately |
| AI provider | AI suggestions and search by meaning disappear (`ai_suggestions`, `semantic_search`); every manual route remains, and search still finds the words typed. One breaker **per endpoint**, because a provider is per tenant. `/meta/health` names no endpoint or model: `disabled` until this process has called a provider, then `ok`, `down`, or `degraded` with `ai.embedding_too_wide`. An installation with no provider reports `ok` with no degraded feature | The feature is greyed out with a reason; with no provider at all there is no control, which the client reads from `/meta/capabilities` |
| External search index (optional) | Fallback to PostgreSQL full-text search | Slower, slightly different search |
| NATS (optional) | The breaker opens, the outbox holds the events, and the publish jobs retry on the queue's ladder; delivery resumes when the bus returns, without a restart | No visible change |
| Webhook recipient | Retries over 24 h, then dead letter plus a subscription warning | A warning in the integration settings |
| OIDC provider | Existing sessions continue (tokens until expiry), local accounts work | New sign-in through that provider is not possible, with a clear message |

The rule: **no failure of an optional dependency may block the core write path.** RT-1 tests it
against a stopped container for each optional dependency (§12).

---

## 8. Data integrity and restart

* **The transaction boundary is the aggregate boundary.** The outbox entry is created in the same
  transaction as the business change: no event without a state change, and vice versa.
* **No external calls inside transactions.**
* **Migrations** are forward only and expand/contract, backwards compatible for at least one minor
  version ([versioning-release.md](./versioning-release.md) §4).
* **Backup:** PITR (WAL archiving) is the documented standard, `pg_dump` the minimal variant for
  self-hosting; the media bucket separately. A restore drill is a **release criterion**, not a
  document ([backup-restore.md](./backup-restore.md)).
* **Post-restore verification:** a consistency check (orphaned items, outbox backlog, migration
  state, tenant isolation).
* **Two safety nets for deletion:** trash for 30 days, then a hard delete; archiving is permanent
  and restorable. An operator error is not data loss.

---

## 9. Zero-downtime operation

* Rolling update and the migration Job before the rollout, with an advisory lock and idempotently:
  [deployment.md](./deployment.md) §2.2, §5.
* Graceful shutdown: `SIGTERM` → mark not ready → keep serving for the deregistration window →
  drain in-flight requests → release job leases → exit; `terminationGracePeriodSeconds` covers the
  longest job timeout plus the drain.
* Schema drift between pods is visible as `hubtask_migration_version` and alerted as A-13. A pod
  does not refuse readiness on a schema mismatch yet ([deployment.md](./deployment.md) §8, D-5).
* Leader tasks (scheduler, outbox dispatcher) use advisory-lock leader election; if the leader
  fails, another takes over within a tick (§15).

---

## 10. Alert catalogue

Alerts are symptom-based, each with a runbook. **The rule files and the runbooks are the
catalogue**: condition, threshold and severity live in
[`deploy/observability/alerts/`](../../deploy/observability/alerts/), meaning and action in
[`deploy/observability/runbooks/`](../../deploy/observability/runbooks/). Every rule carries its
catalogue ID (A-01…A-20) as the label `alert_id` and its runbook as the annotation `runbook`. The
thresholds are starting values; an operator tunes them in their own copy.

**Which file an alert is in is a decision about who is paged** (§11):

* **self-hosting** (`prometheus-rules.yaml`) — the set where doing nothing loses data or leaves the
  installation broken. It is pinned in `test/observability`, so adding to it is a deliberate act.
* **tenant** (`prometheus-rules-tenant.yaml`) — the capacity signal a provider adds; a self-hoster
  who configures no quotas has no series for it.
* **provider** (`prometheus-rules-provider.yaml`) — the rest, for an operator with an on-call rota.
  A-01/A-02 are multiwindow burn rates over recorded `hubtask:slo1_error_ratio:*` series.
* **pitr** (`prometheus-rules-pitr.yaml`) — over the database operator's own series, loaded only
  where CloudNativePG runs; the chart renders it as a `PrometheusRule` where it owns the database.

An alert never fires for a feature that was never installed: A-20 carries no `absent()`, and the
never-ran case is a ticket in the pitr file. Besides the rules, `/meta/health`'s warnings are
visible without any Prometheus.

---

## 11. Dashboards and runbooks

Shipped under `deploy/observability/` — the dashboards (`overview`, `pipeline`, `tenant`, `slo`),
the four rule files of §10 and one runbook per alert (symptom, immediate action, diagnostic query,
escalation, follow-up) — and copied into the chart by `make chart-files` (`make gate-chart` checks
the copy for drift). `tenant.json` sits behind the tenant label of §3.2 and degrades rather than
empties.

A provider loads the self-hosting, tenant and provider files; a self-hoster only the first. The
pitr file belongs to whoever runs CloudNativePG: a rule reading a series nothing emits is silent.

**`make gate-observability` enforces the catalogue:**

* **Any alert without a runbook does not ship**, checked in both directions — an alert whose runbook
  is missing, and a runbook no alert points at. `make gate-selftest` proves it catches an alert added
  without one.
* `promtool check rules` checks every expression; `promtool test rules` drives every alert's
  condition from crafted series and matches its labels and annotations in full, one test file per
  rule file. The burn pair proves the negative too: an outage that ended fires neither A-01 nor
  A-02. An expected annotation includes the trailing newline a YAML `>` block leaves: take it from
  promtool's "got" output rather than typing it.
* The dashboards: the shipped set is the one this section names, no two share a uid, `slo.json`
  has a row for every objective of §2, and every `tenant.json` panel reading `tenant_id` names the
  setting that fills it.

A `promtool` test invents its input, so it cannot prove that the database operator publishes the
series the pitr file reads. `scripts/pitr-drill.sh` (`make gate-pitr`, nightly) scrapes a real
CloudNativePG instance and fails on a name missing from the scrape.

---

## 12. Test evidence for reliability

| Test | Contents | When |
|---|---|---|
| RT-1 Dependency failure | The test container for S3, SMTP and AI (serving the provider's wire format, not a model) is stopped: the core stays writable, `degraded_features` is correct, recovery happens without a restart | PR |
| RT-2 Database outage and return | Pause PostgreSQL: no panic, `readyz` red, and after it returns operation resumes without a restart | PR |
| RT-3 Process death mid-job | `SIGKILL` during job processing: the lease expires, and the job takes effect exactly once | PR |
| RT-4 Duplicate delivery | An event delivered twice: no duplicate effect | PR |
| RT-5 Slow third-party system | A webhook target with 30 s latency: API latency unchanged, the breaker opens | PR |
| RT-6 Overload | Load beyond capacity: load shedding engages, P95 on the interactive paths stays within target, no OOM | Nightly (`make gate-load`) |
| RT-7 Automation loop | A rule pair A↔B: the causality bound stops it, the rule is disabled, the alert metric rises | PR |
| RT-8 Rolling update | A deployment with the N−1/N schema under load: no `5xx`, no data loss | Nightly |
| RT-9 Restore | A point-in-time recovery to a moment between two writes, then the consistency and isolation checks | Per release, in the cluster as a release hook and a weekly `CronJob`; the path itself nightly in `make gate-pitr` on kind with a real CloudNativePG operator and object store ([backup-restore.md](./backup-restore.md) §8.5) |
| RT-10 Clock jump / DST | The scheduler across a time change and after a 2 h outage: no double and no missed firing | PR |
| RT-11 Memory leak test | 1 h of sustained load: `GOMEMLIMIT` held, the goroutine count stable | Nightly |
| RT-12 Observability completeness | Every use case produces a metric plus a span; reconciled against the use case registry | PR (gate) |

RT-12 makes a new feature without signals a red build. Where each test runs is
[ci-cd.md](./ci-cd.md) §3.2.

---

## 13. Operating profiles

| Aspect | Self-hosting | Provider |
|---|---|---|
| Instances | 1 process (all roles) + PostgreSQL | Separate deployments per role, ≥ 2 replicas |
| Backup | A documented `pg_dump` beside the Compose stack ([backup-restore.md](./backup-restore.md) §8.6) | PITR plus a verified restore |
| Alerting | The `/meta/health` warnings; optionally the self-hosting rule file | All rule files plus an on-call rota |
| Tracing | Off (default) | On, with sampling |
| Availability target | "Keeps running, restarts cleanly" | SLOs with an error budget |

The code is identical; only configuration and operating process differ.

### 13.1 Our own operation

The environments this project runs alert through **Prometheus and Alertmanager in the cluster
itself**, applied by `deploy/integration/bootstrap.sh` from
[`deploy/integration/monitoring.yaml`](../../deploy/integration/monitoring.yaml) into a `monitoring`
namespace: Prometheus pinned to the Makefile's `PROMTOOL_VERSION`, the shipped rule files mounted
as a ConfigMap built from `deploy/observability/alerts/`, Alertmanager routing by `severity` (a page
waits for nothing, a ticket groups for five minutes, an info repeats daily), delivery by SMTP into a
mail catcher in the same namespace, and `ALERTS{alertstate="firing"}` kept 45 days.

* **What runs is what is tested.** The rules are not transcribed, and Prometheus is the version
  `make gate-observability` checks them with.
* **The recipient is whoever works on the environment;** it is not a pager. A real installation
  replaces the smarthost with a mail server and the mailbox with a rota; the routing does not
  change.
* **A dead man's switch.** `HubtaskAlertingPathAlive` fires permanently and is delivered every
  twelve hours. Every other alert's silence means nothing until that one has arrived.
* **The environment's own two rules are not in the catalogue.** The watchdog and the scrape check
  (`up{job="hubtask"} == 0`) carry no `alert_id`: §10 is the product's catalogue, and these two are
  this cluster watching itself.
* A-12 fires permanently on an environment that keeps no backups by decision; it is routed to a
  receiver that sends nothing rather than silenced by hand.

### 13.2 Capacity

**Load figures are internal.** They are measured and recorded, and nothing is published until the
release tier has run on named hardware and the figures are stable. The same holds for the RPO and
RTO a restore drill measures. The rules of measurement:

* **The figure a provider can price** is requests per second per vCPU at a held P95, and its decay
  with items per tenant.
* **A concurrent-user count is a derived figure** — throughput divided by a behaviour model — and
  appears only beside that model, never as a headline.
* **Two tiers.** A relative regression guard in the nightly compares a run against a stored
  baseline with an explicit noise band and answers only "did this get significantly worse"; a full
  capacity ramp runs per release on named hardware (the integration server). A shared runner varies
  10–30 % between runs and is not asked a percent-level question ([ci-cd.md](./ci-cd.md) §7).
* **A number names the run that produced it; a number no run produced is written *not measured*,
  never estimated.**

What is measured so far is a laptop's over a toy dataset (5 000 items in 10 tenants, one process
serving every role): [RT-6's overload run](../archive/evidence/RT-6-2026-09-02.md) — 62.4 req/s per
vCPU at a held interactive P95 of 94 ms, shedding engaged with no interactive request refused — and
[the nightly baseline](../../test/load/baselines/steady-state.json), an interactive P95 of 16 ms at
200 req/s. Memory per process, the connections a size needs, storage per item and the items per
tenant at which any of these knees are *not measured*. The chart ships one `values.yaml` and no
per-size presets; its defaults agree with everything measured.

**How a cell gets filled.** The release tier ([`test/load/README.md`](../../test/load/README.md)),
once per release on the integration server: `scripts/seed-load-dataset.sh --items 2000000
--tenants 200`, then `make gate-load` with `HUBTASK_LOAD_HARDWARE=integration`, written up under
`docs/evidence/` with its JSON.

---

## 14. Open points

| # | Point | Needed by |
|---|---|---|
| O-3 | Derive a public status page from `/meta/health` | After `1.0.0` |
| O-5 | Set `GOMEMLIMIT` below the container's memory limit in the chart (derived from the role's limit) and document it for Compose (§6) | Before `1.0.0` |

---

## 15. Jobs and the scheduler

The rule of [ADR-0008](../adr/ADR-0008-jobs-and-scheduling.md): background work runs on a job queue
in PostgreSQL, and nothing else is required to run it.

* **The queue** is the `job` table (`run_at`, `state`, `attempts`, `dedupe_key`). Workers claim
  batches with `SELECT … FOR UPDATE SKIP LOCKED`, so any number of `worker` processes claim
  disjoint work. A job can be written in the same transaction as the business change that causes it.
* **A claim is a lease**: `HUBTASK_JOB_TIMEOUT` plus 30 seconds. A job outliving its lease is
  claimed again, so every job is **idempotent** and effects are guarded by `dedupe_key`.
* **Failures** retry with exponential backoff and full jitter, up to `HUBTASK_JOB_MAX_ATTEMPTS`;
  then the job goes to the dead letter with the code of its last failure, visible in the API and
  alerted as A-07.
* **The scheduler** runs in exactly one active process: the `scheduler` role elects a leader with
  `pg_try_advisory_lock`, acts every `HUBTASK_SCHEDULER_TICK_INTERVAL`, and distributes the work
  itself as jobs. A standby takes over within a tick of the leader's lock being released.
* **Nothing enumerates tenants.** Every per-tenant duty is seeded by its own tenant's write and
  reschedules itself while work remains ([multi-tenancy.md](./multi-tenancy.md) §2.1).
* **Recurrence** is RFC 5545 RRULE through `rrule-go`, read in a stored IANA time zone, and only on
  the server. Occurrences are materialised for a rolling window (90 days by default). Overlapping
  passes lock disjoint series, and the watermark moves under a compare-and-set, so an occurrence is
  never created twice.
* The `queue` port allows a broker adapter later; the queue depth and occurrence lag are metrics
  (§4).
