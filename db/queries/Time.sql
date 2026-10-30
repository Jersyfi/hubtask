-- Time records, timers, weeks, kinds of work and the time policy (migration 0246, M7).

-- name: ListTimeWorkTypes :many
SELECT id, key, name, billable, archived, position, version
FROM time_work_type
WHERE tenant_id = current_tenant_id()
ORDER BY position, key
LIMIT 200;

-- name: UpsertTimeWorkType :exec
INSERT INTO time_work_type (id, tenant_id, key, name, billable, archived, position, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), normalize(sqlc.arg('name')::text, NFC),
  sqlc.arg('billable'), sqlc.arg('archived'), sqlc.arg('position'), sqlc.arg('at'), sqlc.arg('at'), 1)
ON CONFLICT (tenant_id, key) DO UPDATE SET
  name = EXCLUDED.name, billable = EXCLUDED.billable, archived = EXCLUDED.archived,
  position = EXCLUDED.position, updated_at = EXCLUDED.updated_at, version = time_work_type.version + 1;

-- name: InsertTimeRecord :exec
INSERT INTO time_record (id, tenant_id, account_id, entry_id, work_type, day, started_at, minutes,
  note, billable, source, corrects, created_at, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.narg('entry_id'),
  sqlc.arg('work_type'), sqlc.arg('day'), sqlc.narg('started_at'), sqlc.arg('minutes'),
  normalize(sqlc.arg('note')::text, NFC), sqlc.arg('billable'), sqlc.arg('source'),
  sqlc.narg('corrects'), sqlc.arg('created_at'), sqlc.arg('created_at'), 1);

-- name: UpdateTimeRecord :execrows
UPDATE time_record SET
  entry_id = sqlc.narg('entry_id'), work_type = sqlc.arg('work_type'), day = sqlc.arg('day'),
  started_at = sqlc.narg('started_at'), minutes = sqlc.arg('minutes'),
  note = normalize(sqlc.arg('note')::text, NFC), billable = sqlc.arg('billable'),
  updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: DeleteTimeRecord :execrows
DELETE FROM time_record
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: FindTimeRecord :one
SELECT id, account_id, entry_id, work_type, day, started_at, minutes, note, billable, source,
  corrects, created_at, updated_at, version
FROM time_record
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListTimeRecords :many
-- Records in a range of days, of one person or everybody, on one entry or any, in a collection or
-- a hub or anywhere, with each entry's place and title; a purged entry's record has no place.
SELECT r.id, r.account_id, r.entry_id, r.work_type, r.day, r.started_at, r.minutes, r.note,
  r.billable, r.source, r.corrects, r.created_at, r.updated_at, r.version,
  wi.collection_id, c.parent_id AS hub_id, coalesce(wi.title, '')::text AS entry_title
FROM time_record r
LEFT JOIN work_item wi ON wi.tenant_id = r.tenant_id AND wi.id = r.entry_id
LEFT JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
WHERE r.tenant_id = current_tenant_id()
  AND r.day BETWEEN sqlc.arg('from_day')::date AND sqlc.arg('to_day')::date
  AND (sqlc.narg('account_id')::uuid IS NULL OR r.account_id = sqlc.narg('account_id')::uuid)
  AND (sqlc.narg('entry_id')::uuid IS NULL OR r.entry_id = sqlc.narg('entry_id')::uuid)
  AND (sqlc.narg('collection_id')::uuid IS NULL OR wi.collection_id = sqlc.narg('collection_id')::uuid)
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.parent_id = sqlc.narg('hub_id')::uuid)
  AND (sqlc.narg('billable')::boolean IS NULL OR r.billable = sqlc.narg('billable')::boolean)
ORDER BY r.day, r.account_id, r.created_at, r.id
LIMIT sqlc.arg('max')::int;

-- name: SumTimeLines :many
-- The same records summed per person, entry, kind of work, day and billable flag: what a report
-- groups further.
SELECT r.account_id, r.entry_id, wi.collection_id, c.parent_id AS hub_id, r.work_type, r.day,
  r.billable, sum(r.minutes)::bigint AS minutes, count(*)::bigint AS records
FROM time_record r
LEFT JOIN work_item wi ON wi.tenant_id = r.tenant_id AND wi.id = r.entry_id
LEFT JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
WHERE r.tenant_id = current_tenant_id()
  AND r.day BETWEEN sqlc.arg('from_day')::date AND sqlc.arg('to_day')::date
  AND (sqlc.narg('account_id')::uuid IS NULL OR r.account_id = sqlc.narg('account_id')::uuid)
  AND (sqlc.narg('collection_id')::uuid IS NULL OR wi.collection_id = sqlc.narg('collection_id')::uuid)
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.parent_id = sqlc.narg('hub_id')::uuid)
  AND (sqlc.narg('billable')::boolean IS NULL OR r.billable = sqlc.narg('billable')::boolean)
GROUP BY r.account_id, r.entry_id, wi.collection_id, c.parent_id, r.work_type, r.day, r.billable
ORDER BY r.day, r.account_id, r.entry_id
LIMIT sqlc.arg('max')::int;

-- name: SumTimeOnEntries :many
-- Every person's recorded minutes on each of the entries.
SELECT entry_id, sum(minutes)::bigint AS minutes
FROM time_record
WHERE tenant_id = current_tenant_id() AND entry_id = ANY(sqlc.arg('entry_ids')::uuid[])
GROUP BY entry_id;

-- name: SumTimeOfWeek :one
SELECT coalesce(sum(minutes), 0)::bigint AS minutes
FROM time_record
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid
  AND day BETWEEN sqlc.arg('from_day')::date AND sqlc.arg('to_day')::date;

-- name: FindTimeTimer :one
SELECT id, account_id, entry_id, work_type, note, billable, started_at, version
FROM time_timer
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid;

-- name: InsertTimeTimer :exec
INSERT INTO time_timer (id, tenant_id, account_id, entry_id, work_type, note, billable, started_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.narg('entry_id'),
  sqlc.arg('work_type'), normalize(sqlc.arg('note')::text, NFC), sqlc.arg('billable'),
  sqlc.arg('started_at'), 1);

-- name: DeleteTimeTimer :execrows
DELETE FROM time_timer WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid;

-- name: FindTimesheet :one
SELECT id, account_id, week_start, state, minutes, submitted_at, decided_at, decided_by, reason, version
FROM timesheet
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid
  AND week_start = sqlc.arg('week_start')::date;

-- name: ListTimesheets :many
-- Weeks in a state, of one person or everybody, from a first Monday to a last, the oldest first.
SELECT id, account_id, week_start, state, minutes, submitted_at, decided_at, decided_by, reason, version
FROM timesheet
WHERE tenant_id = current_tenant_id()
  AND (sqlc.arg('state')::text = '' OR state = sqlc.arg('state')::text)
  AND (sqlc.narg('account_id')::uuid IS NULL OR account_id = sqlc.narg('account_id')::uuid)
  AND week_start BETWEEN sqlc.arg('from_day')::date AND sqlc.arg('to_day')::date
ORDER BY week_start, account_id
LIMIT sqlc.arg('max')::int;

-- name: InsertTimesheet :exec
INSERT INTO timesheet (id, tenant_id, account_id, week_start, state, minutes, submitted_at,
  decided_at, decided_by, reason, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('account_id'), sqlc.arg('week_start'),
  sqlc.arg('state'), sqlc.arg('minutes'), sqlc.narg('submitted_at'), sqlc.narg('decided_at'),
  sqlc.narg('decided_by'), normalize(sqlc.arg('reason')::text, NFC), sqlc.arg('updated_at'), 1);

-- name: UpdateTimesheet :execrows
UPDATE timesheet SET
  state = sqlc.arg('state'), minutes = sqlc.arg('minutes'), submitted_at = sqlc.narg('submitted_at'),
  decided_at = sqlc.narg('decided_at'), decided_by = sqlc.narg('decided_by'),
  reason = normalize(sqlc.arg('reason')::text, NFC), updated_at = sqlc.arg('updated_at'),
  version = version + 1
WHERE tenant_id = current_tenant_id() AND account_id = sqlc.arg('account_id')::uuid
  AND week_start = sqlc.arg('week_start')::date AND version = sqlc.arg('expected_version')::int;

-- name: FindTimePolicy :one
SELECT id, locked_through, visibility, updated_at, updated_by, version
FROM time_policy
WHERE tenant_id = current_tenant_id();

-- name: InsertTimePolicy :exec
INSERT INTO time_policy (id, tenant_id, locked_through, visibility, updated_at, updated_by, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.narg('locked_through'), sqlc.arg('visibility'),
  sqlc.arg('updated_at'), sqlc.narg('updated_by'), 1);

-- name: UpdateTimePolicy :execrows
UPDATE time_policy SET
  locked_through = sqlc.narg('locked_through'), visibility = sqlc.arg('visibility'),
  updated_at = sqlc.arg('updated_at'), updated_by = sqlc.narg('updated_by'), version = version + 1
WHERE tenant_id = current_tenant_id() AND version = sqlc.arg('expected_version')::int;
