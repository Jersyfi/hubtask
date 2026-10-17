-- name: ListPlans :many
-- The plans not removed, oldest first (multi-tenancy.md §4.2). No tenant predicate: the table
-- carries no policy, and the transaction reading it has no tenant at all.
SELECT id, name, display_name, version, created_at, updated_at
FROM plan
WHERE removed_at IS NULL
ORDER BY created_at, id;

-- name: FindPlanByName :one
-- One plan by its short name, not removed.
SELECT id, name, display_name, version, created_at, updated_at
FROM plan
WHERE name = sqlc.arg('name') AND removed_at IS NULL;

-- name: PlanSettings :many
-- What the given plans decide, in the installation level's key shape.
SELECT plan_id, key, value, locked
FROM plan_setting
WHERE plan_id = ANY(sqlc.arg('plan_ids')::uuid[])
ORDER BY plan_id, key;

-- name: PlanWorkspaceCounts :many
-- How many workspaces are on each plan, through the control plane's own enumerator: `tenant` is
-- behind tenant_self, and admin_tenants() is the one listing across it (multi-tenancy.md §2.1).
SELECT coalesce(plan_name, '')::text AS plan_name, count(*)::bigint AS workspaces
FROM admin_tenants()
WHERE plan_name IS NOT NULL
GROUP BY plan_name;

-- name: InsertPlan :execrows
-- A new plan. A short name taken - also by a removed plan - is a unique violation the adapter
-- answers as `admin.plan_name_taken`.
INSERT INTO plan (id, name, display_name, version, created_at, updated_at, updated_by)
VALUES (
  sqlc.arg('id'), sqlc.arg('name'), sqlc.arg('display_name'), 1,
  sqlc.arg('now'), sqlc.arg('now'), sqlc.narg('updated_by')
)
ON CONFLICT (name) DO NOTHING;

-- name: UpdatePlan :execrows
-- The plan's own row, guarded on the version read: zero rows means somebody saved first, or the
-- plan is gone.
UPDATE plan
SET display_name = sqlc.arg('display_name'), version = version + 1,
    updated_at = sqlc.arg('now'), updated_by = sqlc.narg('updated_by')
WHERE id = sqlc.arg('id') AND removed_at IS NULL
  AND (sqlc.narg('version')::integer IS NULL OR version = sqlc.narg('version')::integer);

-- name: ClearPlanSettings :exec
-- What the write did not decide stops being decided: `PUT` semantics, as the installation's level.
DELETE FROM plan_setting WHERE plan_id = sqlc.arg('plan_id');

-- name: PutPlanSetting :exec
INSERT INTO plan_setting (plan_id, key, value, locked)
VALUES (sqlc.arg('plan_id'), sqlc.arg('key'), sqlc.arg('value'), sqlc.arg('locked'));

-- name: RemovePlan :one
-- Through the one door that can see which workspaces are on the plan (migration 0164).
SELECT remove_plan(sqlc.arg('id'), sqlc.arg('now'), sqlc.narg('actor'))::text AS outcome;

-- name: HoldPlanByName :one
-- The plan a workspace is moving onto, held against a removal until the move commits: a removal
-- waits on this lock, then counts the workspace (remove_plan).
SELECT id
FROM plan
WHERE name = sqlc.arg('name') AND removed_at IS NULL
FOR SHARE;

-- name: AnyPlanEver :one
-- Whether any plan was ever created, removed ones included: what a seeding file asks, so that a
-- plan an operator removed after the seed is not written again at the next start.
SELECT EXISTS (SELECT 1 FROM plan)::boolean AS ever;

-- name: PlanOfWorkspace :one
-- The transaction's own workspace's plan, read in that workspace's scope: `tenant_self` admits its
-- own row, and `plan` carries no policy (multi-tenancy.md §4.2).
SELECT p.id, p.name, p.display_name, p.version, p.created_at, p.updated_at
FROM tenant t
JOIN plan p ON p.id = t.plan_id
WHERE t.id = current_tenant_id() AND t.deleted_at IS NULL;
