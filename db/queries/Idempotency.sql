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
-- A reservation whose work failed, let go (G-09). Without this, a failed action's claim would
-- survive the run that recorded the failure - and a replay of that run would find the key taken
-- and "complete" the action without ever performing it.
DELETE FROM idempotency_key
WHERE key = $1 AND endpoint = $2;
