# Observability and Reliability

The goal: the system runs stably, degrades in a controlled way instead of crashing, loses no data,
and **always knows for itself what it is missing**. Complements [arc42.md](./arc42.md) §8.10/§8.11
and [ADR-0016](../adr/ADR-0016-observability-reliability.md).

---

## 1. Principles

| Principle | Meaning |
|---|---|
| **Self-diagnosis before alerting** | The process knows the state of every dependency and makes it machine-readably available (`/readyz`, `/meta/health`). The operator has to guess nothing. |
| **Degrade rather than die** | The failure of an optional dependency (S3, SMTP, AI, search, NATS) reduces functionality — it terminates no process and blocks no write path. |
| **No silent failure** | Every discarded operation produces a metric plus a log plus, where it matters to the business, a visible state on the object. "Failed and nobody knows" is a bug. |
| **Alert on symptoms** | Alerts hang off user impact (an SLO violation, a backlog), not off CPU utilisation. |
| **Restart is the normal case** | Every process may die at any time. State lives in PostgreSQL, jobs are idempotent and at-least-once. |
| **Observability is part of the Definition of Done** | A use case without a metric, without a trace span, and without error classification is not finished. |

---

## 2. Service level objectives

Reference values for provider operation; in self-hosting the same metrics apply, but without the
error budget process.

| SLO | Indicator | Target | Window |
|---|---|---|---|
| SLO-1 API availability | The share of requests without a `5xx` and without a timeout | ≥ 99.9% | 30 days rolling |
| SLO-2 Read latency | P95 `GET`/`:query` | < 200 ms | 30 days |
| SLO-3 Write latency | P95 mutating operations | < 300 ms | 30 days |
| SLO-4 Event delivery | Outbox lag P99 | < 30 s | 7 days |
| SLO-5 Reminder punctuality | The share of reminders within 60 s of their target time | ≥ 99% | 30 days |
| SLO-6 Webhook delivery | The share successfully delivered (after retries, excluding 4xx recipient errors) | ≥ 99.5% | 7 days |
| SLO-7 Automation | The share of rule runs without an internal error | ≥ 99.5% | 7 days |
| SLO-8 Data loss | Confirmed writes that get lost | **0** | Always |
| RPO / RTO | The data loss and recovery targets after a total outage | RPO ≤ 5 min (PITR), RTO ≤ 60 min | Per incident |

The error budget rule: once 50% of an SLO's budget is consumed, stability work takes precedence
over new features until it recovers. This rule lives in the repository, not just in somebody's head.

---

## 3. Signals

### 3.1 Logs
Structured JSON through `log/slog`. Mandatory fields: `ts`, `level`, `msg`, `service`, `role`,
`version`, `request_id`, `trace_id`, `span_id`, `tenant_id`, `actor_type`, `use_case`,
`error_code`. **No** user content (titles, notes, comments, attachment names), no tokens, no email
addresses in clear text (hashed, or the account ID). Level policy: `ERROR` only for states that
require human action — otherwise `WARN`. Expected business errors (validation, `404`) are `INFO`.

### 3.2 Metrics
OpenTelemetry → a Prometheus endpoint on the internal port (9090, not public).
Label cardinality is bounded: **never** `item_id`, `user_id`, or `rule_id` as a label;
`tenant_id` only if the operator enables it (`HUBTASK_METRICS_TENANT_LABEL=true`) — in provider
operation with many tenants it would otherwise explode the cardinality.

### 3.3 Traces
OpenTelemetry; the W3C `traceparent` is adopted from incoming requests and passed on to outbound
calls — including across the outbox and the job queue (the `trace_id` is persisted in the
event/job). That makes a chain of *HTTP request → event → automation rule → webhook* traceable
end to end. Sampling: 100% of errors, 100% of slow requests (> 1 s), otherwise configurable
(5% by default).

### 3.4 Business events
`activity_entry` and `rule_run` are observability visible to users: why was this task moved? Which
rule fired, and what did it do? That view is part of the product, not just of operations.

---

## 4. Metric catalogue (minimum scope)

| Metric | Type | Labels | Purpose |
|---|---|---|---|
| `hubtask_http_requests_total` | Counter | `route`, `method`, `status_class` | RED rate/errors |
| `hubtask_http_request_duration_seconds` | Histogram | `route`, `method` | RED duration, SLO-2/3 |
| `hubtask_usecase_total` | Counter | `use_case`, `result` (see below) | The business error rate per use case |
| `hubtask_inflight_requests` | Gauge | `role` | Overload detection |
| `hubtask_db_pool_connections` | Gauge | `state` (`in_use`/`idle`) | Saturation |
| `hubtask_db_query_duration_seconds` | Histogram | `query_name` (from sqlc) | Slow queries |
| `hubtask_db_errors_total` | Counter | `class` (`timeout`/`serialization`/`connection`) | Instability |
| `hubtask_outbox_pending` | Gauge | — | Backlog |
| `hubtask_outbox_lag_seconds` | Histogram | — | SLO-4 |
| `hubtask_job_queue_depth` | Gauge | `job_type` | Backlog |
| `hubtask_job_duration_seconds` | Histogram | `job_type` | Runtime behaviour |
| `hubtask_job_failures_total` | Counter | `job_type`, `attempt_class` | Error situation |
| `hubtask_job_dead_letter_total` | Counter | `job_type` | Finally failed → always visible |
| `hubtask_scheduler_tick_lag_seconds` | Gauge | — | The scheduler is stuck |
| `hubtask_reminder_delivery_delay_seconds` | Histogram | `channel` | SLO-5 |
| `hubtask_rule_runs_total` | Counter | `result`, `trigger_type` | SLO-7 |
| `hubtask_rule_disabled_total` | Counter | `reason` | Makes self-protection visible |
| `hubtask_webhook_deliveries_total` | Counter | `result` (`ok`/`retry`/`dead`), `status_class` | SLO-6 |
| `hubtask_webhook_retry_backlog` | Gauge | — | Backlog |
| `hubtask_outbound_http_duration_seconds` | Histogram | `target_class` | Third-party latency |
| `hubtask_circuit_breaker_state` | Gauge | `dependency` | 0 closed / 1 half / 2 open |
| `hubtask_dependency_up` | Gauge | `dependency` | Self-diagnosis as a time series |
| `hubtask_degraded_mode` | Gauge | `feature` | Which feature is currently restricted |
| `hubtask_auth_failures_total` | Counter | `reason` | A security and misconfiguration signal |
| `hubtask_rate_limited_total` | Counter | `scope` | Overload/abuse |
| `hubtask_panics_recovered_total` | Counter | `component` | **must be permanently 0** |
| `hubtask_config_invalid_total` | Counter | `key` | Misconfiguration instead of guesswork |
| `hubtask_build_info` | Gauge (1) | `version`, `commit`, `go_version` | The version situation across the cluster |
| `hubtask_migration_version` | Gauge | — | Makes schema drift between pods visible |
| `hubtask_tenant_quota_usage_ratio` | Gauge | `quota` | Approaching a quota |

### 4.1 The `result` label

`result` is `ok`, or **the error category of the domain error model in lower case** — one label
value per category, never a summary of several. Today that is `validation`, `not_found`,
`conflict`, `forbidden`, `unauthenticated`, `gone`, `rate_limited`, `unavailable`, `internal`. The
set is defined by `core/domain/model/shared.Category`, not by this table: it grows when the error
model grows a category, which is a deliberate act rather than an accident, and the code derives
the label rather than translating it.

The rule follows from §1. **No silent failure** says every discarded operation produces a signal —
and a throttled request counted as `internal` does produce one, it just reports a defect that did
not happen. **Alert on symptoms** then makes that concrete: an alert on "our fault" would page
when a tenant hits the rate limit it was given, which is a system working exactly as configured.
And a use case is not finished without an *error classification*; a classification that contradicts
the domain's own is a second, competing truth about the same failure.

Coarser views are a query, not a label:

```promql
# Our fault - the error budget of SLO-1
sum(rate(hubtask_usecase_total{result=~"internal|unavailable"}[5m])) by (use_case)

# The caller's fault - expected business outcomes, never an alert
sum(rate(hubtask_usecase_total{result=~"validation|not_found|gone|conflict|forbidden|unauthenticated"}[5m]))
```

Aggregating at read time costs a regular expression. Aggregating at write time costs the
information, permanently — a label written coarsely cannot be refined afterwards, and the incident
that needed the distinction is the one where nobody can go back and get it.

Cardinality stays bounded, which is the constraint this rule has to respect (§3.2): ten values is
a closed set fixed by the domain, and a counter series exists only once it has been incremented,
so outcomes that never occur cost nothing. The enemy of cardinality is the unbounded label — an
item ID, a raw path — not the tenth value of an enumeration.

---

## 5. The health model

Four levels, deliberately kept separate:

| Endpoint | Semantics | Who uses it |
|---|---|---|
| `/healthz` | The process is alive, the event loop responds. **Checks no dependencies** — otherwise a database outage kills every pod at once. | Liveness probe |
| `/startupz` | Initialisation complete, the migration state is compatible | Startup probe |
| `/readyz` | The process can serve traffic: the database is reachable, the pool is not exhausted, the schema is compatible, and it is not shutting down | Readiness probe, load balancer |
| `GET /api/v1/meta/health` (authenticated, admin scope) | **Deep self-diagnosis**: per dependency the status, latency, last error, and circuit breaker state; per feature the degradation state; backlogs (outbox, queue, webhook retries); configuration warnings | Operators, support, a status page |

An example response (abridged):

```json
{
  "status": "degraded",
  "version": "0.4.2",
  "migration": { "applied": 47, "expected": 47, "status": "ok" },
  "dependencies": [
    { "name": "postgres", "required": true,  "status": "ok",   "latency_ms": 3 },
    { "name": "object_storage", "required": false, "status": "down",
      "since": "2026-08-14T09:12:04Z", "last_error_code": "storage.unreachable",
      "impact": ["media.upload", "media.download"] },
    { "name": "smtp", "required": false, "status": "ok", "latency_ms": 41 },
    { "name": "ai_provider", "required": false, "status": "disabled" }
  ],
  "degraded_features": [
    { "feature": "media", "reason_code": "dependency.unavailable", "since": "2026-08-14T09:12:04Z" }
  ],
  "backlogs": { "outbox_pending": 12, "job_queue_depth": 3, "webhook_retry_backlog": 0 },
  "warnings": [ { "code": "config.backup_not_configured", "severity": "warn" } ]
}
```

The `warnings` field is the direct expression of the requirement to "always know what it is
missing": a missing backup configuration, missing SMTP with reminders enabled, missing object
storage with uploads enabled, expired signing keys, approaching token expiries, a pod on a stale
schema version — everything is reported as a code with a severity, not as free text.

---

## 6. Resilience patterns

| Pattern | Binding implementation |
|---|---|
| **Timeouts everywhere** | No `http.Client`, no database query, no job without a deadline. A call without a timeout is a lint error (`noctx`, plus a custom check for `context.Background()` outside `main`). |
| **Context propagation** | `ctx` is passed through; a client abort ends the work (except for an already committed transaction). |
| **Retry with backoff + jitter** | Only for idempotent operations; exponential, capped, with a maximum attempt count; never in the synchronous request path against third-party systems. |
| **Circuit breaker** | Per external dependency (object storage, SMTP, AI, and per webhook target). Open → an immediate error instead of a blocked thread; half-open with a probe. The state appears as a metric and in `/meta/health`. |
| **Bulkheads** | Separate connection and worker pools for API, worker, and automation. A runaway rule must not starve the interactive path; under Kubernetes, separate deployments on top. |
| **Load shedding** | Above a threshold for `inflight_requests`, new *non-interactive* requests (bulk, export, search) are rejected with `503` + `Retry-After`, before latency tips over for everyone. |
| **Rate limits** | Internally too: automation rules have throttles per rule and per tenant. |
| **A queue instead of synchrony** | Everything external goes through the outbox or jobs. A hanging webhook recipient cannot delay an API response. |
| **Idempotency** | `Idempotency-Key` on the outside, `job.dedup_key` on the inside, `delivery_id` for webhooks. At-least-once plus idempotency = effectively exactly-once. |
| **Poison pill protection** | After *n* failed attempts → dead letter with full context, a metric, and admin visibility; the queue stays clear. |
| **Optimistic locking** | A `version` per aggregate, a `409` with a machine-readable conflict instead of data loss through last-write-wins. |
| **Panic recovery** | Middleware per request, a wrapper per job, and a **ban on bare goroutines**: concurrency only through `SafeGo(ctx, name, fn)` with recover plus a metric. An architecture test verifies that `go ` occurs only in `shared/concurrency`. |
| **Memory and resource protection** | `GOMEMLIMIT` set, streaming instead of full buffering for uploads and exports, capped result sets — OOM kills are an architecture defect, not an operations problem. |
| **Clock robustness** | The scheduler catches up (bounded catch-up after an outage) and tolerates time jumps; no assumption that "the tick arrived on time". |

---

## 7. Controlled degradation

| Failed dependency | Behaviour | What the user sees |
|---|---|---|
| PostgreSQL | `/readyz` red, no traffic accepted, reconnection with backoff; **no** process exit | `503` with a message code, `Retry-After` |
| Object storage (S3/MinIO) | Core features normal; upload/download disabled, `degraded_features` set | Attachments temporarily unavailable, tasks work |
| SMTP / push | Notifications stay in the queue and are caught up; no loss | The reminder arrives late, with an in-app notice |
| AI provider | AI suggestions disappear, every manual route remains | The feature is greyed out with a reason |
| External search index (optional) | Fallback to PostgreSQL full-text search | Slower, slightly different search |
| NATS (optional) | Fallback to the database outbox dispatch | No visible change |
| Webhook recipient | Retries over 24 h, then dead letter plus a subscription warning | A warning in the integration settings |
| OIDC provider | Existing sessions continue (tokens until expiry), local accounts work | New sign-in through SSO is not possible, with a clear message |

The rule: **no failure of an optional dependency may block the core write path.**
That is a test case, not an intention.

---

## 8. Data integrity and restart

* **The transaction boundary is the aggregate boundary**; the outbox entry is created in the same transaction as the business change → no event without a state change, and vice versa.
* **No external calls inside transactions** (a lint rule plus a review point).
* **Migrations** are expand/contract and backwards compatible for at least one minor version → a rolling update without downtime, a rollback without data loss.
* **Backup**: PITR (WAL archiving) as the documented standard, `pg_dump` as the minimal variant for self-hosting; the media bucket separately. A restore drill is a **release criterion**, not a document.
* **Post-restore verification**: a consistency check (orphaned items, outbox backlog, migration state, tenant isolation).
* **Two safety nets for deletion**: trash for 30 days, then a hard delete; archiving is permanent and restorable. An operator error is not data loss.

---

## 9. Zero-downtime operation

* Rolling update with `maxUnavailable: 0`, a readiness gate, and a `PodDisruptionBudget`.
* Graceful shutdown: `SIGTERM` → deregister from `/readyz` → a grace period for the load balancer → drain in-flight requests → release job leases → exit. `terminationGracePeriodSeconds` ≥ the longest job timeout.
* The migration job runs before the rollout, with an advisory lock, idempotently.
* Schema drift detection: pods with an incompatible migration state report themselves not ready (instead of writing inconsistently).
* Leader tasks (scheduler, outbox dispatcher) use advisory lock leader election with lease renewal; if the leader fails, another takes over within seconds.

---

## 10. Alert catalogue

Symptom-based, each with a runbook. The thresholds are starting values for provider operation.

| ID | Condition | Severity | Meaning |
|---|---|---|---|
| A-01 | SLO-1 error budget burn rate > 14× (1 h) | page | An acute outage |
| A-02 | SLO-1 burn rate > 6× (6 h) | page | A creeping outage |
| A-03 | `readyz` red on > 30% of instances (5 min) | page | A dependency or the rollout is broken |
| A-04 | `hubtask_panics_recovered_total` > 0 | page | A program defect; the target is constantly 0 |
| A-05 | Outbox lag > 60 s (10 min) | page | Events and automation are stuck |
| A-06 | Job queue depth monotonically rising (15 min) | ticket | Processing cannot keep up |
| A-07 | Dead letter > 0 (per type, 15 min) | ticket | Finally failed operations |
| A-08 | Reminder delay P95 > 120 s | ticket | SLO-5 at risk |
| A-09 | Webhook error rate > 20% (30 min, excluding 4xx recipient errors) | ticket | Delivery problems |
| A-10 | Circuit breaker open > 10 min | ticket | A third-party system persistently disrupted |
| A-11 | Database pool utilisation > 80% (10 min) | ticket | Saturation approaching |
| A-12 | A replication/PITR gap, or a backup older than 24 h | page | Risk of data loss |
| A-13 | Migration versions inconsistent across the cluster > 15 min | ticket | The rollout is stuck |
| A-14 | `hubtask_config_invalid_total` > 0 after startup | ticket | Misconfiguration |
| A-15 | Auth error rate > 5%, or refresh reuse detected | ticket/page | Misconfiguration or an attack |
| A-16 | Automation: rule deactivations > 0 in 1 h | ticket | The loop/error protection kicked in |
| A-17 | A certificate or signing key expires in < 14 days | ticket | Preventive |
| A-18 | A tenant exceeds 90% of a quota | info | Capacity planning |

For **self-hosting** there is a reduced variant: a standard Grafana dashboard and an alert rule file
with A-03, A-04, A-05, A-07, A-12 — plus the warnings from `/meta/health`, which are visible even
without Prometheus.

---

## 11. Dashboards and runbooks

Shipped under `deploy/observability/`:

* `dashboards/overview.json` — RED per route, SLO burn, saturation, version situation *(shipped)*
* `dashboards/pipeline.json` — outbox, jobs, automation, webhooks, reminders *(shipped; automation,
  webhooks and reminders join it as those features arrive)*
* `dashboards/tenant.json` — quotas, top tenants (only with the tenant label enabled) *(with
  multi-tenant operation, `0.6.0`)*
* `alerts/prometheus-rules.yaml` — the alert catalogue as rules *(shipped: the reduced
  self-hosting set A-03, A-04, A-05, A-07, A-12; the full catalogue with provider operation)*
* `runbooks/RB-xx.md` — per alert: the symptom, the immediate action, the diagnostic query, escalation, follow-up *(shipped, one per shipped alert)*

Any alert without a runbook does not ship. That is `make gate-observability`, which checks it in
both directions — an alert whose runbook is missing, and a runbook no alert points at — and
`promtool check rules` for the expressions themselves. `make gate-selftest` proves it catches an
alert added without one.

---

## 12. Test evidence for reliability

| Test | Contents | When |
|---|---|---|
| RT-1 Dependency failure | The test container for S3/SMTP/AI is stopped: the core stays writable, `degraded_features` is correct, recovery happens without a restart | PR |
| RT-2 Database outage and return | Pause PostgreSQL: no panic, `readyz` red, and after it returns, operation resumes automatically without a restart | PR |
| RT-3 Process death mid-job | `SIGKILL` during job processing: the lease expires, and the job takes effect exactly once | PR |
| RT-4 Duplicate delivery | An event delivered twice: no duplicate effect (idempotency) | PR |
| RT-5 Slow third-party system | A webhook target with 30 s latency: API latency unchanged, the breaker opens | PR |
| RT-6 Overload | A load test beyond capacity: load shedding engages, P95 on the interactive paths stays within target, no OOM | Nightly |
| RT-7 Automation loop | A rule pair A↔B: the causality bound stops it, the rule is disabled, the alert metric rises | PR |
| RT-8 Rolling update | A deployment with the N−1/N schema under load: no `5xx`, no data loss | Nightly |
| RT-9 Restore | Import a backup, run the consistency and isolation checks | Per release |
| RT-10 Clock jump / DST | The scheduler across a time change and after a 2 h outage: no double firing and no missed firing | PR |
| RT-11 Memory leak test | 1 h of sustained load: `GOMEMLIMIT` held, the goroutine count stable | Nightly |
| RT-12 Observability completeness | Every use case produces a metric plus a span; reconciled against the use case registry | PR (gate) |

RT-12 is the mechanism that stops observability rotting over time: a new feature without signals →
a red build.

---

## 13. Operating profiles

| Aspect | Self-hosting | Provider |
|---|---|---|
| Instances | 1 process (all roles) + PostgreSQL | Separate deployments per role, ≥ 2 replicas |
| Backup | A documented `pg_dump` cron job in the Compose file, with a warning when it is not configured | PITR plus a verified restore |
| Alerting | The `/meta/health` warnings, optional Prometheus rules | The full catalogue plus an on-call rota |
| Tracing | Off (default) | On, with sampling |
| Availability target | "Keeps running, restarts cleanly" | SLOs with an error budget |

Importantly: the code is identical. Only the configuration and the operating process differ — the
self-diagnosis works in both cases.

---

## 14. Open points

| # | Point | Needed by |
|---|---|---|
| O-1 | Choose the alerting backend for our own operation | `0.6.0` |
| O-2 | A capacity model (items per tenant → resources) from real load data | `0.9.0` |
| O-3 | Derive a public status page from `/meta/health` | After `1.0.0` |
| O-4 | Decide: chaos tests permanently in CI, or nightly only | `0.5.0` |
