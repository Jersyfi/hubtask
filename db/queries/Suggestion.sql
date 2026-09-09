-- What AI proposed, and did not do (J-05).
--
-- Every statement runs inside the transaction wrapper that sets `app.tenant_id`, so the policy
-- underneath answers "which workspace" - `current_tenant_id()` is written on insert rather than
-- passed in, the way every tenant-scoped insert in this schema is.

-- name: RecordSuggestion :exec
INSERT INTO ai_suggestion
  (id, tenant_id, target_type, target_id, kind, status, payload,
   source, model, prompt_id, prompt_version, produced_at, input_digest, created_at)
VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('target_type'), sqlc.arg('target_id'),
  sqlc.arg('kind'), 'PROPOSED', sqlc.arg('payload'), sqlc.arg('source'), sqlc.arg('model'),
  sqlc.arg('prompt_id'), sqlc.arg('prompt_version'), sqlc.arg('produced_at'),
  sqlc.arg('input_digest'), sqlc.arg('created_at')
);

-- name: FindSuggestion :one
SELECT id, target_type, target_id, kind, status, payload, source, model, prompt_id,
       prompt_version, produced_at, input_digest, created_at, decided_at, decided_by, version
FROM ai_suggestion
WHERE id = sqlc.arg('id');

-- name: ListSuggestions :many
-- Keyset over (created_at, id) descending, the ordering every list in this schema uses: a page
-- boundary that repeats a row or skips one is what an offset does when a suggestion is recorded
-- while somebody is reading.
SELECT id, target_type, target_id, kind, status, payload, source, model, prompt_id,
       prompt_version, produced_at, input_digest, created_at, decided_at, decided_by, version
FROM ai_suggestion
WHERE target_type = sqlc.arg('target_type')
  AND target_id = sqlc.arg('target_id')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_size');

-- name: DecideSuggestion :execrows
-- Guarded on the version it was read at: two people answering one proposal is a conflict, and the
-- second must be told rather than silently overwriting the first.
UPDATE ai_suggestion
SET status = sqlc.arg('status'), decided_at = sqlc.arg('decided_at'),
    decided_by = sqlc.arg('decided_by'), version = version + 1
WHERE id = sqlc.arg('id') AND version = sqlc.arg('expected_version');

-- name: CountExpiredSuggestions :one
-- Counted no higher than the ceiling, the notification history's shape: the number is for a run
-- log and an operator, and counting a million rows to write "a lot" into a log is a scan nobody
-- asked for.
SELECT count(*)::bigint FROM (
  SELECT 1 FROM ai_suggestion WHERE created_at < sqlc.arg('cutoff') LIMIT sqlc.arg('ceiling')
) AS due;

-- name: DeleteExpiredSuggestions :execrows
-- Oldest first and in batches, so a long-neglected workspace catches up over several passes
-- instead of holding one transaction over everything it owes.
DELETE FROM ai_suggestion
WHERE id IN (
  SELECT id FROM ai_suggestion AS expired
  WHERE expired.created_at < sqlc.arg('cutoff')
  ORDER BY expired.created_at
  LIMIT sqlc.arg('batch')
);
