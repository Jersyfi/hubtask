-- Planning (migration 0244, M2): a plan's entries as the scheduler reads them, a dependency's
-- timing, an entry's scheduling mode and a plan's baselines. A plan is a collection, or a hub with
-- the collections in it.

-- name: ListPlanEntries :many
-- The entries of a plan that are neither trashed nor archived, with their mode, in a stable order,
-- bounded.
SELECT wi.id, wi.parent_id, wi.collection_id, c.parent_id AS hub_id, wi.assignee_id,
  wi.start_at, wi.due_at, wi.due_date_only, wi.due_time_zone, wi.is_completed, wi.version,
  coalesce(es.mode, 'MANUAL')::text AS mode, coalesce(es.milestone, false)::boolean AS milestone
FROM work_item wi
JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
LEFT JOIN entry_schedule es ON es.tenant_id = wi.tenant_id AND es.entry_id = wi.id
WHERE wi.tenant_id = current_tenant_id() AND wi.deleted_at IS NULL AND wi.archived_at IS NULL
  AND c.deleted_at IS NULL
  AND (wi.collection_id = sqlc.arg('plan_id')::uuid OR c.parent_id = sqlc.arg('plan_id')::uuid)
ORDER BY wi.collection_id, wi.depth, wi.order_key, wi.id
LIMIT sqlc.arg('max')::int;

-- name: ListEntrySchedules :many
SELECT entry_id, mode, milestone, updated_at, version
FROM entry_schedule
WHERE tenant_id = current_tenant_id() AND entry_id = ANY(sqlc.arg('entry_ids')::uuid[]);

-- name: UpsertEntrySchedule :exec
INSERT INTO entry_schedule (tenant_id, entry_id, mode, milestone, updated_at, version)
VALUES (current_tenant_id(), sqlc.arg('entry_id'), sqlc.arg('mode'), sqlc.arg('milestone'),
  sqlc.arg('updated_at'), 1)
ON CONFLICT (tenant_id, entry_id) DO UPDATE SET
  mode = EXCLUDED.mode, milestone = EXCLUDED.milestone, updated_at = EXCLUDED.updated_at,
  version = entry_schedule.version + 1;

-- name: SetEntryLinkTiming :execrows
UPDATE entry_link SET dependency_kind = sqlc.arg('dependency_kind'), lag_days = sqlc.arg('lag_days')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListEntryLinkTimings :many
SELECT id, dependency_kind, lag_days
FROM entry_link
WHERE tenant_id = current_tenant_id() AND id = ANY(sqlc.arg('ids')::uuid[]);

-- name: CountPlanBaselines :one
SELECT count(*)::int AS baselines
FROM plan_baseline
WHERE tenant_id = current_tenant_id() AND plan_id = sqlc.arg('plan_id')::uuid;

-- name: InsertPlanBaseline :exec
INSERT INTO plan_baseline (id, tenant_id, plan_id, name, taken_at, taken_by, entry_count)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('plan_id'), normalize(sqlc.arg('name')::text, NFC),
  sqlc.arg('taken_at'), sqlc.narg('taken_by'), 0);

-- name: CopyPlanIntoBaseline :execrows
-- The plan's entries at this moment, the same set ListPlanEntries reads, bounded.
INSERT INTO plan_baseline_entry (tenant_id, baseline_id, entry_id, start_at, due_at)
SELECT current_tenant_id(), sqlc.arg('baseline_id')::uuid, wi.id, wi.start_at, wi.due_at
FROM work_item wi
JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
WHERE wi.tenant_id = current_tenant_id() AND wi.deleted_at IS NULL AND wi.archived_at IS NULL
  AND c.deleted_at IS NULL
  AND (wi.collection_id = sqlc.arg('plan_id')::uuid OR c.parent_id = sqlc.arg('plan_id')::uuid)
ORDER BY wi.id
LIMIT sqlc.arg('max')::int;

-- name: SetPlanBaselineCount :exec
UPDATE plan_baseline SET entry_count = sqlc.arg('entry_count')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListPlanBaselines :many
SELECT id, plan_id, name, taken_at, taken_by, entry_count
FROM plan_baseline
WHERE tenant_id = current_tenant_id() AND plan_id = sqlc.arg('plan_id')::uuid
ORDER BY taken_at DESC, id;

-- name: FindPlanBaseline :one
SELECT id, plan_id, name, taken_at, taken_by, entry_count
FROM plan_baseline
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListPlanBaselineEntries :many
SELECT entry_id, start_at, due_at
FROM plan_baseline_entry
WHERE tenant_id = current_tenant_id() AND baseline_id = sqlc.arg('baseline_id')::uuid
ORDER BY entry_id;

-- name: DeletePlanBaseline :execrows
DELETE FROM plan_baseline WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;
