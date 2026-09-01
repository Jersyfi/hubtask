<!-- SPDX-License-Identifier: BUSL-1.1 -->
# RB-A06 — A job backlog has done nothing but grow

**Alert:** `HubtaskQueueNotKeepingUp` · **Severity:** ticket · **Catalogue:** A-06

## The symptom

The pending count for one job kind has risen throughout half an hour. Work is being scheduled
faster than it is being done. Nothing is lost — the queue is the durable record — but everything
of this kind is later than it should be, and the gap is widening.

## Immediate action

```bash
# How deep, and which kind?
curl -s localhost:9090/metrics | grep hubtask_job_queue_depth

# Is anything failing rather than merely slow?
psql -c "SELECT kind, state, count(*) FROM job GROUP BY kind, state ORDER BY kind;"

# Are workers claiming at all?
psql -c "SELECT kind, count(*), min(run_at) FROM job
         WHERE state = 'RUNNING' GROUP BY kind;"
```

* **Nothing RUNNING** → the `worker` role is missing or down; that is A-03 territory, fix the
  process first.
* **RUNNING but slow** → read `hubtask_job_duration_seconds` for the kind; a duration that
  jumped points at the dependency the handler calls (database, SMTP, a webhook target).
* **One tenant's flood** → the fairness of the claim is round-robin per tenant (H-08); a flood
  slows its own tenant first. If the depth is all one tenant, this is capacity, not defect.

## Diagnostic queries

```promql
hubtask_job_queue_depth
histogram_quantile(0.95, sum(rate(hubtask_job_duration_seconds_bucket[15m])) by (le, job_type))
increase(hubtask_job_failures_total[30m])
```

## Escalation

If the backlog contains reminders (`reminder.fire`), punctuality (SLO-5) is at risk — treat it
with A-08's urgency rather than this ticket's.

## Follow-up

A recurring A-06 on the same kind is a scaling signal: more workers, or a handler that needs to
get cheaper. Record the depth-vs-time shape in the issue; capacity planning (O-2) wants it.
