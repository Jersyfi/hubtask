# RB-A18 — A workspace approaches a quota

**Alert**: `HubtaskTenantQuotaApproaching` — `hubtask_tenant_quota_usage_ratio > 0.9` for 15
minutes. Severity `info`: capacity planning, not an outage. Nothing is broken yet, and that is
the point of being told now.

## What is happening

A workspace stands above 90% of one of its §4 ceilings (multi-tenancy.md). The `quota` label
says which wall: `items`, `media_bytes`, `webhook_targets`, `export_jobs`,
`automation_runs_per_hour`, or `api_requests_per_minute`. When it reaches the wall, the refusal
is `capacity.<quota>` (422) for capacity rows and `429` for the rate — visible to the
workspace's own people, with the ceiling in the answer.

## Who it is

With `HUBTASK_METRICS_TENANT_LABEL=true` the series carries `tenant_id`. Without it, ask the
installation: every workspace's standing is one authenticated call to `GET /quotas` (their own
view), or read the ledger —

```sql
SELECT tenant_id, metric, period, value FROM usage_record ORDER BY period DESC, value DESC LIMIT 20;
```

## What to do

1. Decide whether the growth is legitimate. The audit trail's `tenant.quotas_changed` entries
   say who last moved this wall and when.
2. Raise the ceiling where it is: `PATCH /admin/tenants/{tenantId}/quotas` with the quota's new
   value (0 = unlimited, `null` = back to the mode's default). The write is audited.
3. If the growth is a runaway automation, its runs are already visibly `THROTTLED` in the rule's
   run log — the rule, not the quota, is the thing to fix.

## Threshold

0.9 for 15m is a starting value; an operator with headroom raises the `for`, not the ratio — a
workspace at 95% for a minute is a batch import, at 92% for an hour it is growth.
