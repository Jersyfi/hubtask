-- The §4 limits (multi-tenancy.md, H-08): the overrides in the tenant's own settings document,
-- the live counts the capacity quotas are measured against, and the billing ledger's first
-- writer. Every statement is bounded to the transaction's tenant by row level security; the one
-- naming a tenant column explicitly says why in place.

-- name: TenantQuotaOverrides :one
-- The quotas key of the settings document, whole: the adapter parses it, the application layer
-- never learns the document's shape (TenantPolicy's discipline).
SELECT coalesce(settings->'quotas', '{}'::jsonb) FROM tenant WHERE id = current_tenant_id();

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

-- name: AddUsage :exec
-- The billing ledger's first writer (usage_record has been dormant since phase 0): daily
-- tallies for capacity planning and the dashboards, never the enforcement's source - a ledger
-- row can lag, and a limit that lags is a limit that lies.
INSERT INTO usage_record (tenant_id, period, metric, value)
VALUES (current_tenant_id(), sqlc.arg('period'), sqlc.arg('metric'), sqlc.arg('amount'))
ON CONFLICT (tenant_id, period, metric)
DO UPDATE SET value = usage_record.value + EXCLUDED.value;
