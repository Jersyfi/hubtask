-- Business calendars, SLA policies and an entry's SLA record (migration 0240, M5). Every statement
-- runs under the tenant's context (rule 3); row level security keeps them to it.

-- name: InsertBusinessCalendar :exec
INSERT INTO business_calendar (id, tenant_id, key, name, spec, created_at, updated_at, version)
VALUES (sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('key'), sqlc.arg('name'), sqlc.arg('spec'),
        sqlc.arg('now'), sqlc.arg('now'), 1);

-- name: UpdateBusinessCalendar :execrows
UPDATE business_calendar SET
  name = sqlc.arg('name'), spec = sqlc.arg('spec'), updated_at = sqlc.arg('now'), version = version + 1
WHERE key = sqlc.arg('key') AND version = sqlc.arg('expected_version');

-- name: FindBusinessCalendar :one
SELECT id, tenant_id, key, name, spec, created_at, updated_at, version
FROM business_calendar WHERE key = sqlc.arg('key');

-- name: ListBusinessCalendars :many
SELECT id, tenant_id, key, name, spec, created_at, updated_at, version
FROM business_calendar ORDER BY key;

-- name: DeleteBusinessCalendar :execrows
DELETE FROM business_calendar WHERE key = sqlc.arg('key');

-- name: InsertSlaPolicy :exec
INSERT INTO sla_policy (
  id, tenant_id, key, name, calendar_key, entry_types, priority_field, goals, position, enabled,
  created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('key'), sqlc.arg('name'), sqlc.narg('calendar_key'),
  sqlc.arg('entry_types')::text[], sqlc.arg('priority_field'), sqlc.arg('goals'), sqlc.arg('position'),
  sqlc.arg('enabled'), sqlc.arg('now'), sqlc.arg('now'), 1
);

-- name: UpdateSlaPolicy :execrows
UPDATE sla_policy SET
  name = sqlc.arg('name'), calendar_key = sqlc.narg('calendar_key'),
  entry_types = sqlc.arg('entry_types')::text[], priority_field = sqlc.arg('priority_field'),
  goals = sqlc.arg('goals'), position = sqlc.arg('position'), enabled = sqlc.arg('enabled'),
  updated_at = sqlc.arg('now'), version = version + 1
WHERE key = sqlc.arg('key') AND version = sqlc.arg('expected_version');

-- name: FindSlaPolicy :one
SELECT id, tenant_id, key, name, calendar_key, entry_types, priority_field, goals, position, enabled,
       created_at, updated_at, version
FROM sla_policy WHERE key = sqlc.arg('key');

-- name: ListSlaPolicies :many
SELECT id, tenant_id, key, name, calendar_key, entry_types, priority_field, goals, position, enabled,
       created_at, updated_at, version
FROM sla_policy ORDER BY position, key;

-- name: DeleteSlaPolicy :execrows
DELETE FROM sla_policy WHERE key = sqlc.arg('key');

-- One goal's record on an entry, written whole: armed, moved, met or breached.
-- name: UpsertEntrySla :exec
INSERT INTO entry_sla (
  id, tenant_id, item_id, goal, policy_key, target_seconds, standing, armed_at, due_at,
  at_risk_at, met_at, breached_at, updated_at, version
) VALUES (
  sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('item_id'), sqlc.arg('goal'), sqlc.arg('policy_key'),
  sqlc.arg('target_seconds'), sqlc.arg('standing'), sqlc.arg('armed_at'), sqlc.narg('due_at'),
  sqlc.narg('at_risk_at'), sqlc.narg('met_at'), sqlc.narg('breached_at'), sqlc.arg('now'), 1
)
ON CONFLICT (tenant_id, item_id, goal) DO UPDATE SET
  policy_key = EXCLUDED.policy_key, target_seconds = EXCLUDED.target_seconds,
  standing = EXCLUDED.standing, armed_at = EXCLUDED.armed_at, due_at = EXCLUDED.due_at,
  at_risk_at = EXCLUDED.at_risk_at, met_at = EXCLUDED.met_at, breached_at = EXCLUDED.breached_at,
  updated_at = EXCLUDED.updated_at, version = entry_sla.version + 1;

-- name: ListEntrySla :many
SELECT id, tenant_id, item_id, goal, policy_key, target_seconds, standing, armed_at, due_at,
       at_risk_at, met_at, breached_at, updated_at, version
FROM entry_sla WHERE item_id = sqlc.arg('item_id') ORDER BY goal;

-- How the goals armed in a period stand, per policy and goal: met, breached, running, paused.
-- name: ReportEntrySla :many
SELECT policy_key, goal, standing, count(*)::bigint AS entries
FROM entry_sla
WHERE armed_at >= sqlc.arg('from') AND armed_at < sqlc.arg('to')
GROUP BY policy_key, goal, standing
ORDER BY policy_key, goal, standing;

-- The next running goal's due moment of each of a page's entries: the cursor's key of a queue
-- sorted by sla_due (the query compiler's slaDue, written the same way).
-- name: SlaDueOf :many
SELECT item_id, min(due_at)::timestamptz AS due_at
FROM entry_sla
WHERE item_id = ANY(sqlc.arg('item_ids')::uuid[]) AND standing = 'RUNNING'
GROUP BY item_id;

-- name: InsertReplyTemplate :exec
INSERT INTO reply_template (id, tenant_id, key, name, body, created_at, updated_at, version)
VALUES (sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('key'), sqlc.arg('name'), sqlc.arg('body'),
        sqlc.arg('now'), sqlc.arg('now'), 1);

-- name: UpdateReplyTemplate :execrows
UPDATE reply_template SET
  name = sqlc.arg('name'), body = sqlc.arg('body'), updated_at = sqlc.arg('now'), version = version + 1
WHERE key = sqlc.arg('key') AND version = sqlc.arg('expected_version');

-- name: FindReplyTemplate :one
SELECT id, tenant_id, key, name, body, created_at, updated_at, version
FROM reply_template WHERE key = sqlc.arg('key');

-- name: ListReplyTemplates :many
SELECT id, tenant_id, key, name, body, created_at, updated_at, version
FROM reply_template ORDER BY key;

-- name: DeleteReplyTemplate :execrows
DELETE FROM reply_template WHERE key = sqlc.arg('key');

-- name: SaveSatisfactionRating :one
-- One per request: rating again replaces the score and moves the version on.
INSERT INTO satisfaction_rating (id, tenant_id, item_id, score, rated_at, version)
VALUES (sqlc.arg('id'), sqlc.arg('tenant_id'), sqlc.arg('item_id'), sqlc.arg('score'), sqlc.arg('rated_at'), 1)
ON CONFLICT (tenant_id, item_id) DO UPDATE SET
  score = EXCLUDED.score, rated_at = EXCLUDED.rated_at, version = satisfaction_rating.version + 1
RETURNING id, tenant_id, item_id, score, rated_at, version;

-- name: FindSatisfactionRating :one
SELECT id, tenant_id, item_id, score, rated_at, version
FROM satisfaction_rating WHERE item_id = sqlc.arg('item_id');
