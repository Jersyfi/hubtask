-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against. A row can
-- therefore not be written into the wrong tenant even by a caller that wanted to.

-- name: ReserveIdempotencyKey :execrows
INSERT INTO idempotency_key (tenant_id, key, endpoint, request_hash)
VALUES (current_tenant_id(), $1, $2, $3)
ON CONFLICT (tenant_id, key, endpoint) DO NOTHING;

-- name: FindIdempotencyRecord :one
SELECT request_hash, response_code, response_body
FROM idempotency_key
WHERE key = $1 AND endpoint = $2;

-- name: CompleteIdempotencyRecord :exec
UPDATE idempotency_key
SET response_code = $3, response_body = $4
WHERE key = $1 AND endpoint = $2;

-- name: ReleaseIdempotencyKey :exec
-- A reservation whose work failed, let go. Without this, a failed action's claim would
-- survive the run that recorded the failure - and a replay of that run would find the key taken
-- and "complete" the action without ever performing it.
DELETE FROM idempotency_key
WHERE key = $1 AND endpoint = $2;

-- ========================= The retention sweep =========================

-- name: DeleteExpiredIdempotencyKeys :execrows
-- The IDEMPOTENCY_KEY data kind's sweep (data-retention.md §3: anchor `created_at`, one day, never
-- less): the stored answer of a request old enough that nobody may replay it any more. A
-- reservation still waiting for its answer goes the same way - no request runs for a day.
--
-- Batched through a subquery, DeleteExpiredSessions' shape: DELETE takes no LIMIT, and a pass that
-- took every expired row would be a pass nobody can stop. Oldest first, so a backlog drains in the
-- order it built up. The tenant is the transaction's, through row level security.
DELETE FROM idempotency_key
WHERE (key, endpoint) IN (
  SELECT expired.key, expired.endpoint FROM idempotency_key AS expired
  WHERE expired.created_at < sqlc.arg('cutoff')
  ORDER BY expired.created_at
  LIMIT sqlc.arg('batch')
);

-- name: CountExpiredIdempotencyKeys :one
-- What is due, bounded by the batch the pass would take plus one, CountExpiredSessions' reason.
SELECT count(*) FROM (
  SELECT 1 FROM idempotency_key AS expired
  WHERE expired.created_at < sqlc.arg('cutoff')
  LIMIT sqlc.arg('ceiling')
) AS due;
