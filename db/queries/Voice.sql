-- Voice assistants (ADR-0093). The tenant is never a parameter: row level security bounds every
-- statement to the tenant of the running transaction.

-- name: FindVoiceSetting :one
SELECT enabled, default_collection_id, skill_id, updated_at
FROM voice_setting
WHERE tenant_id = current_tenant_id();

-- name: UpsertVoiceSetting :exec
INSERT INTO voice_setting (tenant_id, enabled, default_collection_id, skill_id, updated_at, updated_by)
VALUES (current_tenant_id(), sqlc.arg('enabled'), sqlc.narg('default_collection_id'),
        sqlc.narg('skill_id'), sqlc.arg('updated_at'), sqlc.narg('updated_by'))
ON CONFLICT (tenant_id) DO UPDATE
SET enabled = EXCLUDED.enabled, default_collection_id = EXCLUDED.default_collection_id,
    skill_id = EXCLUDED.skill_id, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by;

-- name: ListVoiceAliases :many
SELECT id, phrase, collection_id, label_id
FROM voice_alias
ORDER BY folded;

-- name: DeleteVoiceAliases :exec
-- Replacing the list is this and the inserts below in one transaction.
DELETE FROM voice_alias WHERE tenant_id = current_tenant_id();

-- name: InsertVoiceAlias :exec
INSERT INTO voice_alias (id, tenant_id, phrase, folded, collection_id, label_id, created_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('phrase'), sqlc.arg('folded'),
        sqlc.arg('collection_id'), sqlc.narg('label_id'), sqlc.arg('created_at'));
