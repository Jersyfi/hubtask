<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A27 — Retention passes have stopped

**Alert:** `HubtaskRetentionStalled` · **Severity:** ticket · **Catalogue:** A-27

## The symptom

Passes of one data kind completed during the last seven days, and none has completed in the last
48 hours (`hubtask_retention_run_duration_seconds_count{data_kind}`, written once per completed
pass, also when it removed nothing). Every workspace's sweep (`retention.sweep`) runs every
`HUBTASK_RETENTION_INTERVAL` (an hour by default), so two days without one is a sweep that
stopped, not one that is slow. Nothing is lost; data of that kind stays past its period, which is
a data-protection matter rather than an outage (data-retention.md §5).

## Immediate action

```promql
sum by (data_kind) (increase(hubtask_retention_run_duration_seconds_count[48h]))
sum by (job_type) (increase(hubtask_job_dead_letter_total[48h]))
```

* **Every kind stopped** → the sweeps are not run at all. Check that the worker and the scheduler
  run (`/meta/health`, A-03), and whether `retention.sweep` jobs went to dead letters (A-07).
* **One kind stopped, the kinds before it go on** → one pass runs the kinds in order and stops at
  the first that fails. Find the failure in the worker's log and in the run log:

  ```sql
  SELECT tenant_id, data_kind, status, started_at, finished_at
    FROM retention_run
   WHERE started_at > now() - interval '2 days' AND status <> 'SUCCEEDED'
   ORDER BY started_at DESC LIMIT 50;
  ```

* **A kind switched off this week** (its rule disabled, or not a hard delete any more) → nothing is
  wrong; the alert ends once the week has passed.

## Afterwards

Once the failing pass completes, the next ones catch up by themselves: each reads on from its
place (`retention_cursor`), oldest first.
