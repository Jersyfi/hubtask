-- The words a person asked a template to be drafted from (P-11).
--
-- Every statement runs inside the transaction wrapper that sets `app.tenant_id`; the policy
-- underneath answers "which workspace", and `current_tenant_id()` is written on insert.

-- name: PutAiRequest :exec
INSERT INTO ai_request (id, tenant_id, asked_by, text, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('asked_by'), sqlc.arg('text'), sqlc.arg('created_at'));

-- name: FindAiRequest :one
SELECT id, asked_by, text, created_at FROM ai_request WHERE id = sqlc.arg('id');

-- name: DeleteAiRequest :execrows
DELETE FROM ai_request WHERE id = sqlc.arg('id');

-- name: DeleteExpiredAiRequests :execrows
-- The safety net under the job's own deletion: a request an abandoned job left behind goes with
-- the suggestions' own sweep, oldest first and in batches.
DELETE FROM ai_request
WHERE id IN (
  SELECT id FROM ai_request AS expired
  WHERE expired.created_at < sqlc.arg('cutoff')
  ORDER BY expired.created_at
  LIMIT sqlc.arg('batch')
);
