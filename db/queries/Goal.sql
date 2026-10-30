-- Goals and insight (migration 0248, M8): periods, goals, linked entries, check-ins, portfolios,
-- status updates and dashboards. Every statement names the tenant through row level security and
-- `current_tenant_id()` (rule 3).

-- name: ListGoalPeriods :many
SELECT id, name, start_on, end_on, state, version
FROM goal_period
WHERE tenant_id = current_tenant_id()
ORDER BY start_on DESC, id
LIMIT sqlc.arg('max')::int;

-- name: FindGoalPeriod :one
SELECT id, name, start_on, end_on, state, version
FROM goal_period
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: InsertGoalPeriod :exec
INSERT INTO goal_period (id, tenant_id, name, start_on, end_on, state, created_at, created_by, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('name'), sqlc.arg('start_on'), sqlc.arg('end_on'),
  'OPEN', sqlc.arg('at'), sqlc.narg('by'), sqlc.arg('at'), 1);

-- name: UpdateGoalPeriod :execrows
UPDATE goal_period SET
  name = sqlc.arg('name'), start_on = sqlc.arg('start_on'), end_on = sqlc.arg('end_on'),
  updated_at = sqlc.arg('at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid AND state = 'OPEN'
  AND version = sqlc.arg('expected_version')::int;

-- name: CloseGoalPeriod :execrows
UPDATE goal_period SET
  state = 'CLOSED', closed_at = sqlc.arg('at'), closed_by = sqlc.narg('by'), updated_at = sqlc.arg('at'),
  version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid AND state = 'OPEN'
  AND version = sqlc.arg('expected_version')::int;

-- name: GradeGoal :execrows
UPDATE goal SET grade = sqlc.arg('grade')::double precision, updated_at = sqlc.arg('at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid AND grade IS NULL;

-- name: ListGoals :many
SELECT id, period_id, hub_id, parent_id, title, description, owner_id, status, confidence,
       key_results, grade, version
FROM goal
WHERE tenant_id = current_tenant_id()
  AND (sqlc.narg('period_id')::uuid IS NULL OR period_id = sqlc.narg('period_id')::uuid)
  AND (sqlc.narg('parent_id')::uuid IS NULL OR parent_id = sqlc.narg('parent_id')::uuid)
  AND (sqlc.narg('owner_id')::uuid IS NULL OR owner_id = sqlc.narg('owner_id')::uuid)
  AND (cardinality(sqlc.arg('ids')::uuid[]) = 0 OR id = ANY (sqlc.arg('ids')::uuid[]))
ORDER BY created_at, id
LIMIT sqlc.arg('max')::int;

-- name: FindGoal :one
SELECT id, period_id, hub_id, parent_id, title, description, owner_id, status, confidence,
       key_results, grade, version
FROM goal
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: InsertGoal :exec
INSERT INTO goal (id, tenant_id, period_id, hub_id, parent_id, title, description, owner_id, status,
  confidence, key_results, created_at, created_by, updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('period_id'), sqlc.narg('hub_id'), sqlc.narg('parent_id'),
  sqlc.arg('title'), sqlc.arg('description'), sqlc.narg('owner_id'), sqlc.arg('status'), -1,
  sqlc.arg('key_results')::jsonb, sqlc.arg('at'), sqlc.narg('by'), sqlc.arg('at'), sqlc.narg('by'), 1);

-- name: UpdateGoal :execrows
UPDATE goal SET
  period_id = sqlc.arg('period_id'), parent_id = sqlc.narg('parent_id'), title = sqlc.arg('title'),
  description = sqlc.arg('description'), owner_id = sqlc.narg('owner_id'), status = sqlc.arg('status'),
  confidence = sqlc.arg('confidence'), key_results = sqlc.arg('key_results')::jsonb,
  updated_at = sqlc.arg('at'), updated_by = sqlc.narg('by'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: ReparentGoals :exec
UPDATE goal SET parent_id = sqlc.narg('new_parent_id')::uuid, version = version + 1
WHERE tenant_id = current_tenant_id() AND parent_id = sqlc.arg('parent_id')::uuid;

-- name: DeleteGoal :execrows
DELETE FROM goal
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: ListGoalEntries :many
SELECT goal_id, entry_id
FROM goal_entry
WHERE tenant_id = current_tenant_id() AND goal_id = ANY (sqlc.arg('goal_ids')::uuid[])
ORDER BY goal_id, created_at, entry_id;

-- name: ListGoalsOfEntry :many
SELECT goal_id
FROM goal_entry
WHERE tenant_id = current_tenant_id() AND entry_id = sqlc.arg('entry_id')::uuid
ORDER BY created_at, goal_id
LIMIT sqlc.arg('max')::int;

-- name: LinkGoalEntry :exec
INSERT INTO goal_entry (tenant_id, goal_id, entry_id, created_at, created_by)
VALUES (current_tenant_id(), sqlc.arg('goal_id'), sqlc.arg('entry_id'), sqlc.arg('at'), sqlc.narg('by'))
ON CONFLICT DO NOTHING;

-- name: UnlinkGoalEntry :execrows
DELETE FROM goal_entry
WHERE tenant_id = current_tenant_id() AND goal_id = sqlc.arg('goal_id')::uuid
  AND entry_id = sqlc.arg('entry_id')::uuid;

-- name: InsertGoalCheckIn :exec
INSERT INTO goal_check_in (id, tenant_id, goal_id, author_id, status, confidence, note, "values", created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('goal_id'), sqlc.narg('author_id'), sqlc.arg('status'),
  sqlc.arg('confidence'), sqlc.arg('note'), sqlc.arg('values')::jsonb, sqlc.arg('at'));

-- name: ListGoalCheckIns :many
SELECT id, goal_id, author_id, status, confidence, note, "values", created_at
FROM goal_check_in
WHERE tenant_id = current_tenant_id() AND goal_id = sqlc.arg('goal_id')::uuid
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('max')::int;

-- name: ListPortfolios :many
SELECT id, parent_id, name, description, owner_id, status, version
FROM portfolio
WHERE tenant_id = current_tenant_id()
ORDER BY name COLLATE hubtask_name, id
LIMIT sqlc.arg('max')::int;

-- name: FindPortfolio :one
SELECT id, parent_id, name, description, owner_id, status, version
FROM portfolio
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListPortfolioItems :many
SELECT portfolio_id, container_id, position, start_on, end_on, status
FROM portfolio_item
WHERE tenant_id = current_tenant_id() AND portfolio_id = ANY (sqlc.arg('portfolio_ids')::uuid[])
ORDER BY portfolio_id, position;

-- name: InsertPortfolio :exec
INSERT INTO portfolio (id, tenant_id, parent_id, name, description, owner_id, status, created_at, created_by,
  updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.narg('parent_id'), sqlc.arg('name'), sqlc.arg('description'),
  sqlc.narg('owner_id'), sqlc.arg('status'), sqlc.arg('at'), sqlc.narg('by'), sqlc.arg('at'), sqlc.narg('by'), 1);

-- name: UpdatePortfolio :execrows
UPDATE portfolio SET
  parent_id = sqlc.narg('parent_id'), name = sqlc.arg('name'), description = sqlc.arg('description'),
  owner_id = sqlc.narg('owner_id'), status = sqlc.arg('status'), updated_at = sqlc.arg('at'),
  updated_by = sqlc.narg('by'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: ClearPortfolioItems :exec
DELETE FROM portfolio_item
WHERE tenant_id = current_tenant_id() AND portfolio_id = sqlc.arg('portfolio_id')::uuid;

-- name: InsertPortfolioItem :exec
INSERT INTO portfolio_item (tenant_id, portfolio_id, container_id, position, start_on, end_on, status)
VALUES (current_tenant_id(), sqlc.arg('portfolio_id'), sqlc.arg('container_id'), sqlc.arg('position'),
  sqlc.narg('start_on'), sqlc.narg('end_on'), sqlc.arg('status'));

-- name: ReparentPortfolios :exec
UPDATE portfolio SET parent_id = sqlc.narg('new_parent_id')::uuid, version = version + 1
WHERE tenant_id = current_tenant_id() AND parent_id = sqlc.arg('parent_id')::uuid;

-- name: DeletePortfolio :execrows
DELETE FROM portfolio
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: InsertPortfolioUpdate :exec
INSERT INTO portfolio_update (id, tenant_id, portfolio_id, author_id, status, summary, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('portfolio_id'), sqlc.narg('author_id'), sqlc.arg('status'),
  sqlc.arg('summary'), sqlc.arg('at'));

-- name: ListPortfolioUpdates :many
SELECT id, portfolio_id, author_id, status, summary, created_at
FROM portfolio_update
WHERE tenant_id = current_tenant_id() AND portfolio_id = sqlc.arg('portfolio_id')::uuid
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('max')::int;

-- name: ListDashboards :many
SELECT id, hub_id, owner_id, name, sharing, group_ids, widgets, version
FROM dashboard
WHERE tenant_id = current_tenant_id()
ORDER BY name COLLATE hubtask_name, id
LIMIT sqlc.arg('max')::int;

-- name: FindDashboard :one
SELECT id, hub_id, owner_id, name, sharing, group_ids, widgets, version
FROM dashboard
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: InsertDashboard :exec
INSERT INTO dashboard (id, tenant_id, hub_id, owner_id, name, sharing, group_ids, widgets, created_at, updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.narg('hub_id'), sqlc.narg('owner_id'), sqlc.arg('name'),
  sqlc.arg('sharing'), coalesce(sqlc.arg('group_ids')::uuid[], '{}'), sqlc.arg('widgets')::jsonb, sqlc.arg('at'), sqlc.arg('at'), sqlc.narg('owner_id'), 1);

-- name: UpdateDashboard :execrows
UPDATE dashboard SET
  name = sqlc.arg('name'), sharing = sqlc.arg('sharing'), group_ids = coalesce(sqlc.arg('group_ids')::uuid[], '{}'),
  widgets = sqlc.arg('widgets')::jsonb,
  updated_at = sqlc.arg('at'), updated_by = sqlc.narg('by'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: DeleteDashboard :execrows
DELETE FROM dashboard
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: FindDashboardDigest :one
SELECT id, dashboard_id, account_id, cadence, weekday, hour, time_zone, next_at, last_sent_at
FROM dashboard_digest
WHERE tenant_id = current_tenant_id() AND dashboard_id = sqlc.arg('dashboard_id')::uuid
  AND account_id = sqlc.arg('account_id')::uuid;

-- name: FindDashboardDigestByID :one
SELECT id, dashboard_id, account_id, cadence, weekday, hour, time_zone, next_at, last_sent_at
FROM dashboard_digest
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: PutDashboardDigest :one
INSERT INTO dashboard_digest (id, tenant_id, dashboard_id, account_id, cadence, weekday, hour, time_zone,
  next_at, created_at, updated_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('dashboard_id'), sqlc.arg('account_id'), sqlc.arg('cadence'),
  sqlc.arg('weekday'), sqlc.arg('hour'), sqlc.arg('time_zone'), sqlc.arg('next_at'), sqlc.arg('at'), sqlc.arg('at'))
ON CONFLICT (tenant_id, dashboard_id, account_id) DO UPDATE SET
  cadence = EXCLUDED.cadence, weekday = EXCLUDED.weekday, hour = EXCLUDED.hour,
  time_zone = EXCLUDED.time_zone, next_at = EXCLUDED.next_at, updated_at = EXCLUDED.updated_at
RETURNING id;

-- name: DeleteDashboardDigest :execrows
DELETE FROM dashboard_digest
WHERE tenant_id = current_tenant_id() AND dashboard_id = sqlc.arg('dashboard_id')::uuid
  AND account_id = sqlc.arg('account_id')::uuid;

-- name: MarkDashboardDigestSent :execrows
UPDATE dashboard_digest SET last_sent_at = COALESCE(sqlc.narg('sent_at'), last_sent_at), next_at = sqlc.arg('next_at')
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;
