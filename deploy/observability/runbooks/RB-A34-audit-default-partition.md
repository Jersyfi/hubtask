<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A34 — Audit entries are stuck in the default partition

**Alert:** `HubtaskAuditDefaultPartitionAged` · **Severity:** ticket · **Catalogue:** A-34

## The symptom

For six hours the trail's default partition has held entries past their retention period
(`hubtask_audit_default_partition_aged_rows`, sampled by the scheduler's leader). Retention drops
whole monthly partitions; an entry in the default partition is in none of them, so it is kept
beyond its period. Entries land there when they were written for a month whose partition did not
exist yet - after a clock jump, or while the leader's partition maintenance did not run.

## Immediate action

See which months the rows are for:

```sql
SELECT date_trunc('month', occurred_at) AS month, count(*)
FROM audit_log_default GROUP BY 1 ORDER BY 1;
```

PostgreSQL refuses to create a month's partition while the default partition holds rows of that
month, so the leader cannot repair this by itself. In one transaction, in a maintenance window:
detach the default partition, create the month's partition (`ensure_audit_partition`), move that
month's rows into the trail (`INSERT INTO audit_log SELECT * FROM audit_log_default WHERE …`,
then delete them from the detached table) and attach the default partition again.

## Afterwards

Once the rows are in their months, the next retention pass removes those past their period and the
gauge drops to zero. The months a legal hold keeps whole (`hubtask_audit_partitions_held`) are the
hold working and raise no alert.
