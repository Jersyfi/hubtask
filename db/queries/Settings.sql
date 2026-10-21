-- The structure layers of hubs, collections and the workspace (domain-model.md §2, "Levels and
-- presets"). A container's own layer is the `structure` key of its policies document, beside
-- `completion_policy`, which is the collection's own completion; the workspace's layer, its locks
-- and its default preset are the `structure` key of the tenant's settings. Both documents are
-- written by key, so every other key in them - and every writer that predates this file - is left
-- alone.

-- name: SetContainerStructure :execrows
-- A container's own layer, and on a collection its own completion: an empty completion removes the
-- key, so the collection follows its hub again. Both in one statement, guarded on the version like
-- every container write (api-guidelines.md §5). The document is bound as a parameter (rule 9).
UPDATE container SET
  policies = CASE
    WHEN sqlc.arg('completion_policy')::text = '' THEN
      jsonb_set(policies - 'completion_policy', '{structure}', sqlc.arg('structure')::jsonb, true)
    ELSE
      jsonb_set(
        jsonb_set(policies, '{structure}', sqlc.arg('structure')::jsonb, true),
        '{completion_policy}', to_jsonb(sqlc.arg('completion_policy')::text), true)
  END,
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid AND version = sqlc.arg('expected_version');

-- name: LockContainerShared :one
-- Holds a hub's row while a collection's structure is resolved from it, so that the hub's own
-- write - which updates this row - waits for that collection's, and the next reads it.
SELECT id FROM container WHERE id = sqlc.arg('id')::uuid FOR SHARE;

-- name: ListStructureHeirs :many
-- The live containers that inherit a layer: a hub's collections, or with no hub every hub and
-- collection of the workspace. Bounded by the caller's ceiling, which it reads one past to know
-- the fan-out is too large to write row by row. The column list is FindContainer's, so the rows
-- map through the same reader.
SELECT
  c.id, c.tenant_id, c.type, c.parent_id, c.name, c.description, c.icon, c.color_token, c.order_key,
  coalesce(c.policies->>'completion_policy', '')::text AS completion_policy,
  coalesce(c.policies->'structure', '{}'::jsonb)::text AS structure,
  aap.strategy AS auto_assign_strategy,
  aap.candidates AS auto_assign_candidates,
  aap.enabled AS auto_assign_enabled,
  c.archived_at, parent.archived_at AS parent_archived_at,
  c.deleted_at, c.trash_batch_id, c.created_by, c.created_at, c.updated_at, c.version,
  c.private
FROM container c
LEFT JOIN container parent ON parent.id = c.parent_id
LEFT JOIN auto_assign_policy aap ON aap.scope_type = 'COLLECTION' AND aap.scope_id = c.id
WHERE c.tenant_id = current_tenant_id()
  AND c.deleted_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.parent_id = sqlc.narg('hub_id')::uuid)
ORDER BY c.id
LIMIT sqlc.arg('ceiling');

-- name: FindWorkspaceStructure :one
-- The workspace's layer and the row's version.
SELECT coalesce(settings->'structure', '{}'::jsonb)::text AS structure, version
FROM tenant WHERE id = current_tenant_id();

-- name: FindWorkspaceStructureForShare :one
-- The same, held while a hub's or a collection's structure is resolved and written from it, so
-- that a workspace write waits for that one and the next reads it.
SELECT coalesce(settings->'structure', '{}'::jsonb)::text AS structure, version
FROM tenant WHERE id = current_tenant_id() FOR SHARE;

-- name: FindWorkspaceStructureForUpdate :one
-- The same, held by the workspace's own write until it has fanned out.
SELECT coalesce(settings->'structure', '{}'::jsonb)::text AS structure, version
FROM tenant WHERE id = current_tenant_id() FOR UPDATE;

-- name: SetWorkspaceStructure :execrows
-- The workspace's layer, written by key under the tenant row's version: two administrators
-- changing it see each other (409), and the sign-in rule's writer keeps its keys.
UPDATE tenant SET
  settings   = jsonb_set(settings, '{structure}', sqlc.arg('structure')::jsonb, true),
  updated_at = sqlc.arg('updated_at'),
  version    = version + 1
WHERE id = current_tenant_id() AND version = sqlc.arg('expected_version');
