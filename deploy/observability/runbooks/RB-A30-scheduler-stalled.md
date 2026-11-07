<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A30 — The scheduler is not running its duties

**Alert:** `HubtaskSchedulerStalled` · **Severity:** page · **Catalogue:** A-30

## The symptom

For ten minutes no instance has led the scheduler on time: the newest
`hubtask_scheduler_last_tick_timestamp_seconds` is more than two minutes old, the leader's
`hubtask_scheduler_tick_lag_seconds` is above thirty seconds, or no instance has reported a tick
at all. While it lasts no backup, reminder, recurrence, retention or privacy deadline is scheduled.
The queue does not grow, so the queue alerts stay quiet.

## Immediate action

* **No instance runs the scheduler role** (a scheduler deployment scaled to zero, `HUBTASK_ROLES`
  without `scheduler`) → start one. One instance leads; the others stand by.
* **The leader hangs** (its log stops at a tick, its lag gauge still reads low) → restart that
  instance; a standby takes the advisory lock within one of its own ticks.
* **Every instance stands by** (`scheduler stands by: the database lacks migrations`) → run the
  migrator; an instance whose schema is behind neither takes nor keeps leadership.
* **The lock's session hangs in the database**:

```sql
SELECT pid, state, wait_event, now() - state_change AS idle_for
FROM pg_stat_activity
WHERE pid IN (SELECT pid FROM pg_locks WHERE locktype = 'advisory');
```

  A session idle in transaction for minutes holds leadership for nobody: `SELECT
  pg_terminate_backend(<pid>);` lets a standby take over.

## Afterwards

The duties catch up at the next ticks: a reminder due while the scheduler stood still fires late
rather than never (A-08), a backup missed is taken at its next window - check A-12 and the backup
list for the gap.
