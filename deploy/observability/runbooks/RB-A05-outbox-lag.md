<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A05 — Events are reaching their consumers late

**Alert:** `HubtaskOutboxLagging` · **Severity:** page · **Catalogue:** A-05 · **SLO:** SLO-4

## The symptom

The age of an event at the moment it reaches its consumers (P99) is above 60 seconds. Nothing is
lost — the outbox is transactional, so the events are all still there — but everything downstream
of them is behind: automation rules, webhook deliveries, the search index, live updates.

The usual cause is not slowness. It is that **nothing is dispatching**: the `worker` role is not
running, or its dispatch jobs are stuck.

**What feeds the histogram, and what does not.** A dispatch round reports the age of every event it
had in hand, whether or not the delivery succeeded — so a consumer that fails every time does raise
this alert (G-02; before that it reported only successful deliveries and the one state where events
are certainly late was the one state nothing measured). What still reports nothing is a dispatcher
that is not running at all, because no round happens: for that case read
`hubtask_job_queue_depth{job_type="outbox.dispatch"}` below, which is a gauge the scheduler
publishes whether or not anything is dispatching.

## Immediate action

```bash
# Is a worker running at all?
docker compose logs app --since 10m | grep -E "worker ready|job failed"

# What is waiting, and since when? (as the owner role - this reads across tenants)
psql -c "SELECT tenant_id, count(*), min(occurred_at)
         FROM outbox_event WHERE dispatched_at IS NULL GROUP BY tenant_id;"

# Is the dispatch job itself alive, or did it give up?
psql -c "SELECT tenant_id, state, attempts, last_error, run_at
         FROM job WHERE kind = 'outbox.dispatch';"
```

Three shapes, three answers:

* **No `outbox.dispatch` row at all** → no event has been written since the queue existed, or the
  job was purged. The next write recreates it; to force one now, restart the process.
* **`state = 'DEAD_LETTER'`** → dispatch failed repeatedly; `last_error` carries the code. Fix the
  cause, then delete the row — the next event in that tenant enqueues a fresh job.
* **`state = 'PENDING'` with `run_at` in the past** → nothing is claiming it: the `worker` role is
  not in `HUBTASK_ROLES`, or the process is not running.

## Diagnostic queries

```promql
histogram_quantile(0.99, sum(rate(hubtask_outbox_lag_seconds_bucket[10m])) by (le))
hubtask_job_queue_depth{job_type="outbox.dispatch"}
increase(hubtask_job_failures_total{job_type="outbox.dispatch"}[15m])
```

## Escalation

None. If the lag is genuine backlog (a bulk import), it drains by itself — the dispatcher chases a
backlog without waiting between rounds. Watch the P99 fall rather than intervening.

## Follow-up

A lag caused by a dead-lettered dispatch job means a consumer error was not retryable. That is
worth an issue: at-least-once delivery is supposed to survive a failing consumer, not stop on it.
