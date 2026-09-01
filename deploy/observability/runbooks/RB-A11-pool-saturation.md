<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A11 — A connection pool is running near its ceiling

**Alert:** `HubtaskPoolNearSaturation` · **Severity:** ticket · **Catalogue:** A-11

## The symptom

More than 80% of one pool's connections have been in use for ten minutes. Nothing is failing
yet — this is the warning before the waiting starts. The `pool` label says which budget is
being spent: the API pool serves requests with a seconds-long query budget, the background pool
serves jobs with a minute-long one, and they are separate precisely so that this alert names
the culprit (observability-reliability.md §6).

## Immediate action

```bash
# Who is holding connections, and for how long?
psql -c "SELECT state, count(*), max(now() - query_start) FROM pg_stat_activity
         WHERE application_name LIKE 'hubtask%' GROUP BY state;"

# The long-runners themselves (no user content in the query text - sqlc names are constants):
psql -c "SELECT pid, now() - query_start AS held, left(query, 60) FROM pg_stat_activity
         WHERE state <> 'idle' ORDER BY held DESC LIMIT 10;"
```

* **API pool, many short queries** → genuine load; this is capacity, see whether the rate
  matches `hubtask_http_requests_total` growth or one tenant's burst.
* **Background pool, few long holds** → a job is holding a transaction open around something
  slow. The design forbids holding one around an external call — if `pg_stat_activity` shows a
  connection idle-in-transaction for minutes, that is a defect; note the job kind from the logs.
* **Either pool, `AcquireWait` climbing** → the ceiling is genuinely too low for this
  installation; raising `HUBTASK_DB_MAX_CONNS` (per role) is legitimate, PostgreSQL permitting.

## Diagnostic queries

```promql
sum by (pool, state) (hubtask_db_pool_connections)
histogram_quantile(0.95, sum(rate(hubtask_db_query_duration_seconds_bucket[10m])) by (le)) # future: not yet emitted
hubtask_inflight_requests
```

## Escalation

If saturation reaches 100% the API starts queueing acquisitions and requests slow down before
they fail — expect A-01/A-02 to follow if it is the API pool. Treat 100% as a page even though
this alert tickets.

## Follow-up

A recurring A-11 without matching request growth points at query regressions. The pool ceiling
is a bulkhead, not a dial to keep turning.
