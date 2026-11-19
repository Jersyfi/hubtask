-- An inventory object's own history (migration 0435, UC-AST-01): appended in the transaction of
-- the write it records, read newest first.

-- name: AppendCiHistory :exec
INSERT INTO cmdb_ci_history (id, tenant_id, ci_id, actor_type, actor_id, verb, change_set, occurred_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('ci_id'), sqlc.arg('actor_type'),
        sqlc.narg('actor_id'), sqlc.arg('verb'), sqlc.arg('change_set'), sqlc.arg('occurred_at'));

-- name: ListCiHistory :many
-- One page of an object's history, newest first, keyed on (occurred_at, id) as ListActivity is:
-- two steps of one act share a moment. Served by cmdb_ci_history_page_idx.
SELECT id, ci_id, actor_type, actor_id, verb, change_set, occurred_at
FROM cmdb_ci_history
WHERE ci_id = sqlc.arg('ci_id')
  AND (
    sqlc.narg('cursor_occurred_at')::timestamptz IS NULL
    OR (occurred_at, id) < (sqlc.narg('cursor_occurred_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('page_size');
