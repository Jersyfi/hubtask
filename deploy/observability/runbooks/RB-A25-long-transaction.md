<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A25 — A database transaction stays open for hours

**Alert:** `HubtaskLongTransaction` · **Severity:** ticket · **Catalogue:** A-25

## The symptom

For ten minutes, the oldest open transaction of Hubtask's database has run for more than two
hours (`hubtask_db_oldest_transaction_seconds`, sampled by the scheduler's leader on the primary).
While it stays open, vacuum keeps every row version it might still read: the busy tables - the
job queue, the outbox, the change log - and their indexes grow, and queries slow down. Nothing is
lost, and it ends by itself when the transaction does.

The gauge sees the transactions of the application's own role, the ones Hubtask can hold open.

## Immediate action

```sql
-- Which session, since when, doing what kind of work (no query text needed):
SELECT pid, usename, application_name, backend_type, state,
       now() - xact_start AS open_for, wait_event_type, wait_event
FROM pg_stat_activity
WHERE datname = current_database() AND xact_start IS NOT NULL
ORDER BY xact_start
LIMIT 5;
```

* **A backup, a backup verify, a workspace export or the owner's workspace download** (a
  `REPEATABLE READ` read-only session; the server logs the run) → expected for a large workspace:
  the archive is one point in time (backup-restore.md §5). Let it finish; a download nobody reads
  ends after two minutes by itself, and every run ends at `HUBTASK_LONG_JOB_TIMEOUT` (default 6h).
  If it recurs every night, schedule the backups off-peak.
* **`idle in transaction`** for hours, from a person's `psql` or a tool → end it:
  `SELECT pg_terminate_backend(<pid>);` Ask who opened it first if it is not Hubtask's.
* **Anything else of Hubtask's** → note the `application_name` and `backend_type` for an issue;
  ending it with `pg_terminate_backend` is safe, the job is retried.

## Afterwards

Once it ended, autovacuum catches up on its own. Where a table grew far beyond its rows, a
`VACUUM (VERBOSE)` of it during a quiet hour shows what was reclaimed.
