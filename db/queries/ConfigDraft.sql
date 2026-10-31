-- Drafts of the configuration's shared blocks (migration 0308, ui.md A4).

-- name: SaveConfigDraft :one
-- The first save writes version 1; a later one moves the version only when expected is the stored
-- one, or 0. No row answered is a save over a draft that moved.
INSERT INTO config_draft AS d (tenant_id, section, key, block, saved_by, saved_at, version)
SELECT current_tenant_id(), sqlc.arg('section'), sqlc.arg('key'), sqlc.arg('block')::jsonb,
  sqlc.arg('saved_by'), sqlc.arg('saved_at'), 1
WHERE sqlc.arg('expected_version')::int = 0
   OR EXISTS (
     SELECT 1 FROM config_draft
     WHERE tenant_id = current_tenant_id() AND section = sqlc.arg('section') AND key = sqlc.arg('key')
   )
ON CONFLICT (tenant_id, section, key) DO UPDATE SET
  block = EXCLUDED.block, saved_by = EXCLUDED.saved_by, saved_at = EXCLUDED.saved_at,
  version = d.version + 1
WHERE sqlc.arg('expected_version')::int = 0 OR d.version = sqlc.arg('expected_version')::int
RETURNING d.section, d.key, d.block, d.saved_by, d.saved_at, d.version;

-- name: ListConfigDrafts :many
SELECT section, key, block, saved_by, saved_at, version
FROM config_draft
WHERE tenant_id = current_tenant_id()
ORDER BY section, key
LIMIT 500;

-- name: DiscardConfigDraft :execrows
DELETE FROM config_draft
WHERE tenant_id = current_tenant_id() AND section = sqlc.arg('section') AND key = sqlc.arg('key');

-- name: PublishedConfigDraft :exec
-- Only a draft holding exactly the block applied: one saved again since is kept.
DELETE FROM config_draft
WHERE tenant_id = current_tenant_id() AND section = sqlc.arg('section') AND key = sqlc.arg('key')
  AND block = sqlc.arg('block')::jsonb;
