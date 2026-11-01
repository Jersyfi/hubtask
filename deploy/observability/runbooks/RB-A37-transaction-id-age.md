<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A37 — The database's transaction-ID age is high

**Alert:** `HubtaskTransactionIdAgeHigh` · **Severity:** ticket · **Catalogue:** A-37

## The symptom

For half an hour the database has gone more than a billion transaction IDs without vacuum
freezing it (`hubtask_db_frozen_xid_age`, `age(datfrozenxid)`, sampled by the scheduler's leader).
At about two billion PostgreSQL stops accepting writes until a vacuum has frozen the oldest rows;
on a large database that vacuum takes hours. Nothing is lost yet.

## Immediate action

Find what keeps vacuum from freezing - an old transaction or a replication slot holds the horizon
back, or autovacuum cannot keep up:

```sql
SELECT relname, age(relfrozenxid) AS xid_age, n_dead_tup, last_autovacuum
FROM pg_stat_user_tables JOIN pg_class ON pg_class.oid = relid
ORDER BY age(relfrozenxid) DESC LIMIT 10;

SELECT slot_name, active, age(xmin) FROM pg_replication_slots;
```

* **A transaction open for hours** → see RB-A25; it ends by itself or is ended.
* **An inactive replication slot** → drop it once nobody needs it (`pg_drop_replication_slot`).
* **Autovacuum behind on a busy table** → run `VACUUM (FREEZE, VERBOSE) <table>;` in a quiet hour.

## Afterwards

The age falls once a vacuum has frozen the oldest table; the alert clears when it is below a
billion again.
