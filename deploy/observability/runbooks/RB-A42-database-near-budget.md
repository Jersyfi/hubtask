<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A42 — The database is past 80% of its size budget

**Alert:** `HubtaskDatabaseNearBudget` · **Severity:** ticket · **Catalogue:** A-42

## The symptom

For an hour the database's size on disk (`hubtask_db_size_bytes`, `pg_database_size`, sampled by
the scheduler's leader) has been above 80% of the budget the chart records from
`prometheusRules.databaseSizeBudget` - the size a managed database's plan or the volume allows.
Past the budget a managed database stops accepting writes or a volume fills. Nothing is lost yet.

## Immediate action

Find what grows:

```sql
SELECT relname, pg_size_pretty(pg_total_relation_size(relid)) FROM pg_statio_user_tables
ORDER BY pg_total_relation_size(relid) DESC LIMIT 10;
```

* **The audit or change log** → check that retention runs (RB-A27) and the partitions are pruned.
* **Dead rows after a large deletion** → vacuum reclaims them for reuse; the size falls only after
  `VACUUM FULL` in a maintenance window.
* **Real growth** → raise the plan or the volume, then the budget value.

## Afterwards

The alert clears when the database is below 80% of the budget again.
