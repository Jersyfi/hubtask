-- Sprints and estimates (migration 0245, M2, PLN-06, PLN-07).

-- name: InsertSprint :exec
INSERT INTO sprint (id, tenant_id, collection_id, name, goal, start_on, end_on, state, unit,
  created_at, created_by, updated_at, version)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('collection_id'),
  normalize(sqlc.arg('name')::text, NFC), normalize(sqlc.arg('goal')::text, NFC),
  sqlc.arg('start_on'), sqlc.arg('end_on'), 'PLANNED', sqlc.arg('unit'),
  sqlc.arg('created_at'), sqlc.narg('created_by'), sqlc.arg('created_at'), 1);

-- name: UpdateSprint :execrows
-- Every field a person or a start or close changes, held to the version the caller read.
UPDATE sprint SET
  name = normalize(sqlc.arg('name')::text, NFC), goal = normalize(sqlc.arg('goal')::text, NFC),
  start_on = sqlc.arg('start_on'), end_on = sqlc.arg('end_on'), state = sqlc.arg('state'),
  unit = sqlc.arg('unit'),
  committed_value = sqlc.arg('committed_value'), committed_count = sqlc.arg('committed_count'),
  completed_value = sqlc.arg('completed_value'), completed_count = sqlc.arg('completed_count'),
  started_at = sqlc.narg('started_at'), closed_at = sqlc.narg('closed_at'),
  updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')::int;

-- name: FindSprint :one
SELECT id, collection_id, name, goal, start_on, end_on, state, unit, committed_value,
  committed_count, completed_value, completed_count, started_at, closed_at, created_at,
  created_by, updated_at, version
FROM sprint
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListSprints :many
-- A collection's sprints, the latest first, bounded.
SELECT id, collection_id, name, goal, start_on, end_on, state, unit, committed_value,
  committed_count, completed_value, completed_count, started_at, closed_at, created_at,
  created_by, updated_at, version
FROM sprint
WHERE tenant_id = current_tenant_id() AND collection_id = sqlc.arg('collection_id')::uuid
  AND (sqlc.arg('state')::text = '' OR state = sqlc.arg('state')::text)
ORDER BY start_on DESC, id
LIMIT sqlc.arg('max')::int;

-- name: DeleteSprint :execrows
DELETE FROM sprint WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id')::uuid;

-- name: ListSprintEntries :many
-- A sprint's memberships with each entry's completion, the open ones and its history; a trashed
-- entry is left out.
SELECT se.entry_id, se.added_at, se.removed_at, se.committed, se.outcome,
  wi.is_completed, wi.completed_at, wi.collection_id, c.parent_id AS hub_id
FROM sprint_entry se
JOIN work_item wi ON wi.tenant_id = se.tenant_id AND wi.id = se.entry_id
JOIN container c ON c.tenant_id = wi.tenant_id AND c.id = wi.collection_id
WHERE se.tenant_id = current_tenant_id() AND se.sprint_id = sqlc.arg('sprint_id')::uuid
  AND wi.deleted_at IS NULL
ORDER BY se.added_at, se.entry_id
LIMIT sqlc.arg('max')::int;

-- name: ListOpenSprintMemberships :many
-- Which open sprint each of the entries is in.
SELECT entry_id, sprint_id
FROM sprint_entry
WHERE tenant_id = current_tenant_id() AND entry_id = ANY(sqlc.arg('entry_ids')::uuid[])
  AND removed_at IS NULL AND outcome IS NULL;

-- name: AddSprintEntry :exec
-- An entry that left the sprint and joins it again joins it anew.
INSERT INTO sprint_entry (tenant_id, sprint_id, entry_id, added_at, committed)
VALUES (current_tenant_id(), sqlc.arg('sprint_id'), sqlc.arg('entry_id'), sqlc.arg('added_at'),
  sqlc.arg('committed'))
ON CONFLICT (tenant_id, sprint_id, entry_id) DO UPDATE SET
  added_at = EXCLUDED.added_at, removed_at = NULL, outcome = NULL, committed = EXCLUDED.committed;

-- name: RemoveSprintEntry :execrows
UPDATE sprint_entry SET removed_at = sqlc.arg('removed_at')
WHERE tenant_id = current_tenant_id() AND sprint_id = sqlc.arg('sprint_id')::uuid
  AND entry_id = sqlc.arg('entry_id')::uuid AND removed_at IS NULL AND outcome IS NULL;

-- name: CommitSprintEntries :exec
UPDATE sprint_entry SET committed = true
WHERE tenant_id = current_tenant_id() AND sprint_id = sqlc.arg('sprint_id')::uuid
  AND removed_at IS NULL AND outcome IS NULL;

-- name: SetSprintEntryOutcomes :exec
UPDATE sprint_entry SET outcome = sqlc.arg('outcome')
WHERE tenant_id = current_tenant_id() AND sprint_id = sqlc.arg('sprint_id')::uuid
  AND entry_id = ANY(sqlc.arg('entry_ids')::uuid[]) AND removed_at IS NULL AND outcome IS NULL;

-- name: UpsertEntryEstimate :exec
INSERT INTO entry_estimate (tenant_id, entry_id, unit, value, size, updated_at, updated_by, version)
VALUES (current_tenant_id(), sqlc.arg('entry_id'), sqlc.arg('unit'), sqlc.arg('value'),
  sqlc.narg('size'), sqlc.arg('updated_at'), sqlc.narg('updated_by'), 1)
ON CONFLICT (tenant_id, entry_id) DO UPDATE SET
  unit = EXCLUDED.unit, value = EXCLUDED.value, size = EXCLUDED.size,
  updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by,
  version = entry_estimate.version + 1;

-- name: DeleteEntryEstimate :execrows
DELETE FROM entry_estimate WHERE tenant_id = current_tenant_id() AND entry_id = sqlc.arg('entry_id')::uuid;

-- name: ListEntryEstimates :many
SELECT entry_id, unit, value, size, updated_at, updated_by, version
FROM entry_estimate
WHERE tenant_id = current_tenant_id() AND entry_id = ANY(sqlc.arg('entry_ids')::uuid[]);
