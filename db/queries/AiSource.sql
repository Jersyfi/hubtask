-- The workspace's choice of AI source (ADR-0072 §2, §3). One row per workspace at most; no row is
-- the workspace's own model, which is what every workspace did before anything was offered.

-- name: FindAiSource :one
SELECT tenant_id, source, offered_provider_id, updated_at, version
FROM ai_source;

-- name: UpsertAiSource :one
-- The version rises on every write, so two administrators choosing at once are both in the trail
-- and the later one stands.
INSERT INTO ai_source (tenant_id, source, offered_provider_id, updated_at)
VALUES (current_tenant_id(), sqlc.arg('source'), sqlc.narg('offered_provider_id'), sqlc.arg('now'))
ON CONFLICT (tenant_id) DO UPDATE SET
  source              = excluded.source,
  offered_provider_id = excluded.offered_provider_id,
  updated_at          = excluded.updated_at,
  version             = ai_source.version + 1
RETURNING tenant_id, source, offered_provider_id, updated_at, version;
