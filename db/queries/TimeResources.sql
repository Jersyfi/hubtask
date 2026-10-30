-- A person's capacity and absences, cost rates and budgets (migration 0247, M7).

-- name: ListTimeCapacities :many
-- The capacities of the people named, or of everybody where none is named, by person and day.
SELECT id, account_id, from_day, week_minutes, version
FROM time_capacity
WHERE tenant_id = current_tenant_id()
  AND (cardinality(sqlc.arg('account_ids')::uuid[]) = 0 OR account_id = ANY(sqlc.arg('account_ids')::uuid[]))
ORDER BY account_id, from_day
LIMIT sqlc.arg('max')::int;

-- name: UpsertTimeCapacity :exec
INSERT INTO time_capacity (id, tenant_id, account_id, from_day, week_minutes, updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('from_day'),
  sqlc.arg('week_minutes')::int[], sqlc.arg('at'), sqlc.narg('updated_by'), 1)
ON CONFLICT (tenant_id, account_id, from_day) DO UPDATE SET
  week_minutes = EXCLUDED.week_minutes, updated_at = EXCLUDED.updated_at,
  updated_by = EXCLUDED.updated_by, version = time_capacity.version + 1;

-- name: DeleteTimeCapacity :execrows
DELETE FROM time_capacity
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid
  AND from_day = sqlc.arg('from_day')::date;

-- name: ListTimeAbsences :many
-- The absences that share a day with a range, of one person or everybody, the earliest first.
SELECT id, account_id, from_day, to_day, kind, half, version
FROM time_absence
WHERE tenant_id = current_tenant_id()
  AND from_day <= sqlc.arg('to_day')::date AND to_day >= sqlc.arg('from_day')::date
  AND (sqlc.narg('account_id')::uuid IS NULL OR account_id = sqlc.narg('account_id')::uuid)
ORDER BY from_day, account_id, id
LIMIT sqlc.arg('max')::int;

-- name: FindTimeAbsence :one
SELECT id, account_id, from_day, to_day, kind, half, version
FROM time_absence
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: InsertTimeAbsence :exec
INSERT INTO time_absence (id, tenant_id, account_id, from_day, to_day, kind, half, created_at, created_by, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('from_day'),
  sqlc.arg('to_day'), sqlc.arg('kind'), sqlc.arg('half'), sqlc.arg('at'), sqlc.narg('created_by'),
  sqlc.arg('at'), 1);

-- name: UpdateTimeAbsence :execrows
UPDATE time_absence SET
  from_day = sqlc.arg('from_day'), to_day = sqlc.arg('to_day'), kind = sqlc.arg('kind'),
  half = sqlc.arg('half'), updated_at = sqlc.arg('at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: DeleteTimeAbsence :execrows
DELETE FROM time_absence
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: ListTimeCostRates :many
SELECT id, account_id, role, hour_minor, currency, from_day, to_day, version
FROM time_cost_rate
WHERE tenant_id = current_tenant_id()
ORDER BY account_id NULLS LAST, role, from_day
LIMIT sqlc.arg('max')::int;

-- name: FindTimeCostRate :one
SELECT id, account_id, role, hour_minor, currency, from_day, to_day, version
FROM time_cost_rate
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: InsertTimeCostRate :exec
INSERT INTO time_cost_rate (id, tenant_id, account_id, role, hour_minor, currency, from_day, to_day, updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.narg('account_id'), sqlc.narg('role'),
  sqlc.arg('hour_minor'), sqlc.arg('currency'), sqlc.arg('from_day'), sqlc.narg('to_day'),
  sqlc.arg('at'), sqlc.narg('updated_by'), 1);

-- name: UpdateTimeCostRate :execrows
UPDATE time_cost_rate SET
  hour_minor = sqlc.arg('hour_minor'), currency = sqlc.arg('currency'), from_day = sqlc.arg('from_day'),
  to_day = sqlc.narg('to_day'), updated_at = sqlc.arg('at'), updated_by = sqlc.narg('updated_by'),
  version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: DeleteTimeCostRate :execrows
DELETE FROM time_cost_rate
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: FindTimeBudget :one
SELECT id, plan_id, minutes, amount_minor, currency, version
FROM time_budget
WHERE tenant_id = current_tenant_id() AND plan_id = sqlc.arg('plan_id')::uuid;

-- name: InsertTimeBudget :exec
INSERT INTO time_budget (id, tenant_id, plan_id, minutes, amount_minor, currency, updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('plan_id'), sqlc.arg('minutes'),
  sqlc.arg('amount_minor'), sqlc.arg('currency'), sqlc.arg('at'), sqlc.narg('updated_by'), 1);

-- name: UpdateTimeBudget :execrows
UPDATE time_budget SET
  minutes = sqlc.arg('minutes'), amount_minor = sqlc.arg('amount_minor'), currency = sqlc.arg('currency'),
  updated_at = sqlc.arg('at'), updated_by = sqlc.narg('updated_by'), version = version + 1
WHERE tenant_id = current_tenant_id() AND plan_id = sqlc.arg('plan_id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: DeleteTimeBudget :execrows
DELETE FROM time_budget
WHERE tenant_id = current_tenant_id() AND plan_id = sqlc.arg('plan_id')::uuid
  AND version = sqlc.arg('expected_version')::int;
