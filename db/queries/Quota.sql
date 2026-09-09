-- The §4 limits (multi-tenancy.md, H-08): the overrides in the tenant's own settings document,
-- the live counts the capacity quotas are measured against, and the billing ledger's first
-- writer. Every statement is bounded to the transaction's tenant by row level security; the one
-- naming a tenant column explicitly says why in place.

-- name: TenantQuotaOverrides :one
-- The quotas key of the settings document, whole: the adapter parses it, the application layer
-- never learns the document's shape (TenantPolicy's discipline).
SELECT coalesce(settings->'quotas', '{}'::jsonb)::text FROM tenant WHERE id = current_tenant_id();

-- name: SetTenantQuotas :execrows
-- Replaces the quotas key and only it - require_admin_totp and whatever else lives beside it
-- stay untouched. Guarded on the row version: two operators moving one wall see each other.
UPDATE tenant
SET settings = jsonb_set(settings, '{quotas}', sqlc.arg('quotas')::jsonb, true),
    updated_at = sqlc.arg('now'), version = version + 1
WHERE id = current_tenant_id() AND deleted_at IS NULL
  AND version = sqlc.arg('expected_version');

-- name: CountTenantItems :one
-- Trash included: a row occupies its place until the retention machinery lets it go, and a
-- quota that emptied by trashing would be no wall at all.
SELECT count(*) FROM work_item;

-- name: SumTenantMediaBytes :one
-- Soft-deleted objects excluded: their bytes are the reconciliation job's to reclaim, not the
-- workspace's to answer for.
SELECT coalesce(sum(byte_size), 0)::bigint FROM media_object WHERE deleted_at IS NULL;

-- name: CountWebhookTargets :one
SELECT count(*) FROM webhook_subscription;

-- name: CountTenantRunsSince :one
-- CountRunsSince's reasoning, without its rule predicate: the tenant's whole hour, THROTTLED
-- excluded - counting the refusals would make the bound tighten on itself.
SELECT count(*) FROM rule_run
WHERE started_at >= sqlc.arg('since') AND status <> 'THROTTLED';

-- name: SumTenantUsageSince :one
-- What the workspace has spent on one metered thing since a day (J-15).
--
-- The ledger as an enforcement source, which the statement below is at pains to say it is not -
-- and the exception is the point rather than an oversight. `usage_record` is barred elsewhere
-- because it is a lagging copy of something countable: items are in `work_item`, media in
-- `media_object`. A token has no row. It was spent on somebody else's machine and this tally *is*
-- the record, so there is nothing more authoritative to count.
SELECT coalesce(sum(value), 0)::bigint FROM usage_record
WHERE metric = sqlc.arg('metric') AND period >= sqlc.arg('since');

-- name: AddUsage :exec
-- The billing ledger's first writer (usage_record has been dormant since phase 0): daily
-- tallies for capacity planning and the dashboards, never the enforcement's source - a ledger
-- row can lag, and a limit that lags is a limit that lies.
INSERT INTO usage_record (tenant_id, period, metric, value)
VALUES (current_tenant_id(), sqlc.arg('period'), sqlc.arg('metric'), sqlc.arg('amount'))
ON CONFLICT (tenant_id, period, metric)
DO UPDATE SET value = usage_record.value + EXCLUDED.value;
