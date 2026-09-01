-- The control plane (H-06, multi-tenancy.md §5): provisioning, suspension, the deletion request
-- and the hard delete after the grace.
--
-- Two departures from the query files around this one, both deliberate. The listing goes through
-- the SECURITY DEFINER enumerator migration 0067 pins down - the one legitimate place tenants are
-- enumerated (0.6.0 decision 6). And the statements on the tenant row itself still say
-- current_tenant_id(): the control plane opens an ordinary bounded transaction per tenant it
-- touches, so even here no statement can reach a row its transaction was not opened for.

-- name: AdminTenants :many
-- The casts are for the generator: it cannot see into the function's OUT table.
SELECT id::uuid, slug::text, display_name::text, status::text,
       default_locale::text, default_time_zone::text,
       created_at::timestamptz, purge_after::timestamptz AS purge_after
FROM admin_tenants();

-- name: InsertTenant :exec
-- Runs inside the new tenant's own scope: the tenant_self policy's WITH CHECK compares the row
-- against current_tenant_id(), so the first write a tenant ever sees is already bounded to it.
INSERT INTO tenant
  (id, slug, display_name, status, default_locale, default_time_zone, settings, created_at, updated_at)
VALUES (
  current_tenant_id(), sqlc.arg('slug'), sqlc.arg('display_name'), 'ACTIVE',
  sqlc.arg('default_locale'), sqlc.arg('default_time_zone'), sqlc.arg('settings'),
  sqlc.arg('now'), sqlc.arg('now')
);

-- name: FindTenantForAdmin :one
SELECT id, slug, display_name, status, default_locale, default_time_zone,
       created_at, purge_after, version
FROM tenant
WHERE id = current_tenant_id() AND deleted_at IS NULL;

-- name: SetTenantStatus :execrows
-- The lifecycle edges of §5, one guarded write each: the expected status is the edge's origin,
-- so a suspension of a suspended tenant or a resume of an active one changes zero rows and the
-- caller reports the conflict instead of overwriting a state it never read.
UPDATE tenant
SET status = sqlc.arg('next_status'), purge_after = sqlc.narg('purge_after'),
    updated_at = sqlc.arg('now'), version = version + 1
WHERE id = current_tenant_id() AND deleted_at IS NULL
  AND status = sqlc.arg('expected_status');

-- name: RequestTenantDeletion :execrows
-- The one edge with two origins (§5: Active and Suspended both lead to PendingDeletion).
UPDATE tenant
SET status = 'PENDING_DELETION', purge_after = sqlc.arg('purge_after'),
    updated_at = sqlc.arg('now'), version = version + 1
WHERE id = current_tenant_id() AND deleted_at IS NULL
  AND status IN ('ACTIVE', 'SUSPENDED');

-- name: HardDeleteTenant :execrows
-- The final row. The cascade takes every table with a foreign key; what carries none is removed
-- explicitly around this statement, and the guard re-reads the two facts the grace could have
-- changed - a resumed tenant or a moved deadline makes this delete nothing.
DELETE FROM tenant
WHERE id = current_tenant_id() AND status = 'PENDING_DELETION'
  AND purge_after IS NOT NULL AND purge_after <= sqlc.arg('now');

-- ====================== What the cascade cannot reach ======================

-- name: CountTenantFootprint :one
-- The stores the §5 table names, counted before the delete and again after: the difference is
-- what the evidence entry records, the zeros afterwards are the acceptance proof. Search rides
-- inside work_item (its vectors are columns of the row), so the item count is the search count.
SELECT
  (SELECT count(*) FROM work_item)                                   AS items,
  (SELECT count(*) FROM container)                                   AS containers,
  (SELECT count(*) FROM media_object)                                AS media_objects,
  (SELECT coalesce(sum(byte_size), 0) FROM media_object)::bigint     AS media_bytes,
  (SELECT count(*) FROM outbox_event)                                AS outbox_events,
  (SELECT count(*) FROM audit_log)                                   AS audit_entries;

-- name: ListTenantStorageKeys :many
-- The purge walks the object store with this page: bytes are deleted store-first, rows after,
-- ReconcileMedia's order - an orphaned byte object is reclaimable, an orphaned row lies.
SELECT storage_key FROM media_object
WHERE storage_key > sqlc.arg('after')
ORDER BY storage_key
LIMIT sqlc.arg('batch');

-- name: DeleteTenantOutbox :execrows
-- Bounded by row level security to the tenant of the transaction; outbox_event carries no
-- foreign key, so the cascade never reaches it.
DELETE FROM outbox_event
WHERE id IN (SELECT id FROM outbox_event LIMIT sqlc.arg('batch'));

-- name: DeleteTenantJobs :execrows
-- The job table has no policy (multi-tenancy.md §2.1), so the tenant is an explicit predicate -
-- CancelJob's precedent - taken from the transaction's own scope, like everywhere else. The job
-- running this delete is its own row; it survives to report.
DELETE FROM job
WHERE tenant_id = current_tenant_id() AND id <> sqlc.arg('keep_id');

-- name: DisableAllAutomationRules :execrows
-- The deletion request switches the tenant's automations off in one stroke (§5), visibly: the
-- rows keep existing with enabled = false, the way DisableFailingRule leaves them.
UPDATE automation_rule
SET enabled = false, updated_at = sqlc.arg('now'), version = version + 1
WHERE deleted_at IS NULL AND enabled = true;

-- name: PurgeTenantTrail :one
-- The SECURITY DEFINER act migration 0067 reasons through, aimed by the transaction's own
-- scope: even this narrow path can only take the trail of the tenant it was opened for.
SELECT purge_tenant_trail(current_tenant_id())::bigint AS removed;

-- ====================== The instance's own journal =========================

-- name: InsertInstanceEvent :exec
INSERT INTO instance_event
  (id, occurred_at, action, tenant_id, tenant_slug, actor_label, details)
VALUES (
  sqlc.arg('id'), sqlc.arg('occurred_at'), sqlc.arg('action'), sqlc.narg('tenant_id'),
  sqlc.narg('tenant_slug'), sqlc.narg('actor_label'), sqlc.arg('details')
);

-- name: ListInstanceEvents :many
SELECT id, occurred_at, action, tenant_id, tenant_slug, actor_label, details
FROM instance_event
WHERE tenant_id = sqlc.arg('tenant_id')
ORDER BY occurred_at, id;

-- ====================== The ordered fall of the structure ==================
-- A bare DELETE FROM tenant would trip its own cascade: RESTRICT edges (a hub under its
-- collections, a media object under its covers and attachments, an account under the rules that
-- run as it, a backup target under its runs) are checked per row, in an order nobody controls.
-- The purge therefore fells the structure explicitly, children first, all of it bounded by row
-- level security to the tenant of the transaction - and only then lets the cascade take the rest.

-- name: DeleteTenantCollections :execrows
DELETE FROM container WHERE parent_id IS NOT NULL;

-- name: DeleteTenantHubs :execrows
DELETE FROM container;

-- name: DeleteTenantMediaRows :execrows
-- After the collections: nothing references media once the items are gone. The bytes were
-- deleted store-first before this statement, ReconcileMedia's order.
DELETE FROM media_object;

-- name: DeleteTenantAutomationRules :execrows
-- Before the cascade reaches the accounts its rules run as (the run_as RESTRICT edge).
DELETE FROM automation_rule;

-- name: DeleteTenantRestoreRuns :execrows
-- Before the cascade reaches a tenant-scoped backup target (the target_id RESTRICT edge).
DELETE FROM restore_run;

-- name: DeleteTenantRetentionRules :execrows
DELETE FROM retention_rule;

-- name: DeleteTenantIdempotency :execrows
-- Bounded by row level security; idempotency_key carries no foreign key, so the cascade never
-- reaches it.
DELETE FROM idempotency_key;
