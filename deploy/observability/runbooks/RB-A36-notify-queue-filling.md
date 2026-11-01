<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A36 — PostgreSQL's NOTIFY queue is filling

**Alert:** `HubtaskNotifyQueueFilling` · **Severity:** page · **Catalogue:** A-36

## The symptom

For two minutes more than half of PostgreSQL's NOTIFY queue has been in use
(`hubtask_db_notify_queue_usage_ratio`, `pg_notification_queue_usage()`, sampled by the
scheduler's leader). The queue keeps every notification until the slowest listening session has
read it; the live updates and the awareness relay notify on every change. When it is full, every
transaction that notifies fails at commit - writes to entries and documents among them - with
`could not queue notification`.

## Immediate action

A listening session that stopped reading is the cause, usually one that holds a transaction open:

```sql
SELECT pid, usename, application_name, state, now() - xact_start AS open_for, wait_event
FROM pg_stat_activity
WHERE datname = current_database() AND (state = 'idle in transaction' OR xact_start < now() - interval '5 minutes')
ORDER BY xact_start
LIMIT 10;
```

* **A Hubtask session** (`application_name` `hubtask/…`) → restart that pod; its listener
  reconnects and reads from the current position. A session idle in a transaction for longer than
  its role's bound is ended by the database on its own (`idle_in_transaction_session_timeout`).
* **A session of another tool** that ran `LISTEN` → end it: `SELECT pg_terminate_backend(<pid>);`.

## Afterwards

The queue drains as soon as the slowest listener reads again or is gone, and the gauge falls
back to zero. Writes that failed in the meantime were refused, not half-written; clients retry
them.
