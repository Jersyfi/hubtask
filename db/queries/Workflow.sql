-- Workflows and the scheme (migration 0216, UC-FLW-02 … 05). A workspace's rows only: the shipped
-- workflow a workspace has never used or changed is the domain's and has none.

-- name: ListWorkflows :many
SELECT id, tenant_id, key, system, name, published, draft, published_at, published_by,
  created_at, updated_at, version
FROM workflow
WHERE tenant_id = current_tenant_id()
ORDER BY key;

-- name: InsertWorkflow :exec
INSERT INTO workflow (
  id, tenant_id, key, system, name, published, draft, published_at, published_by,
  created_at, updated_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('key'), sqlc.arg('system'),
  normalize(sqlc.arg('name')::text, NFC), sqlc.arg('published')::jsonb, sqlc.narg('draft')::jsonb,
  sqlc.narg('published_at'), sqlc.narg('published_by'), sqlc.arg('created_at'),
  sqlc.arg('created_at'), 1
);

-- name: UpdateWorkflow :execrows
-- The whole row as the application decided it, under the optimistic lock (api-guidelines.md §5).
-- The key and whether it is the shipped one never change.
UPDATE workflow SET
  name         = normalize(sqlc.arg('name')::text, NFC),
  published    = sqlc.arg('published')::jsonb,
  draft        = sqlc.narg('draft')::jsonb,
  published_at = sqlc.narg('published_at'),
  published_by = sqlc.narg('published_by'),
  updated_at   = sqlc.arg('updated_at'),
  version      = version + 1
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text
  AND version = sqlc.arg('expected_version');

-- name: DeleteWorkflow :execrows
-- Only a workspace's own workflow no scheme names; the reference refuses the others too.
DELETE FROM workflow
WHERE tenant_id = current_tenant_id() AND key = sqlc.arg('key')::text AND NOT system
  AND NOT EXISTS (
    SELECT 1 FROM workflow_scheme s
    WHERE s.tenant_id = current_tenant_id() AND s.workflow_key = sqlc.arg('key')::text
  );

-- name: ListWorkflowSchemes :many
SELECT container_id, entry_type, workflow_key
FROM workflow_scheme
WHERE tenant_id = current_tenant_id()
ORDER BY container_id, entry_type;

-- name: SetWorkflowScheme :exec
INSERT INTO workflow_scheme (tenant_id, container_id, entry_type, workflow_key, created_at)
VALUES (current_tenant_id(), sqlc.arg('container_id'), sqlc.arg('entry_type'),
  sqlc.arg('workflow_key'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, container_id, entry_type) DO UPDATE SET
  workflow_key = EXCLUDED.workflow_key;

-- name: DeleteWorkflowScheme :execrows
DELETE FROM workflow_scheme
WHERE tenant_id = current_tenant_id() AND container_id = sqlc.arg('container_id')::uuid
  AND entry_type = sqlc.arg('entry_type')::text;

-- name: WorkflowStatusGroups :many
-- The entries of some collections counted by type, status and completion: what a publish or a
-- change of scheme moves (work.PlanStatusMoves). Trashed entries count and move too, so that one
-- restored from the trash finds a status its workflow has. Served by wi_status_idx where a status
-- is set and by the collection's index otherwise.
SELECT wi.collection_id,
  coalesce(wi.entry_type, lower(wi.type::text))::text AS entry_type,
  coalesce(wi.status, '')::text AS status,
  wi.is_completed,
  count(*)::int AS count
FROM work_item wi
WHERE wi.tenant_id = current_tenant_id() AND wi.collection_id = ANY(sqlc.arg('collection_ids')::uuid[])
GROUP BY 1, 2, 3, 4
ORDER BY 1, 2, 3, 4;

-- name: MoveWorkItemStatuses :many
-- One group's move: the entries of a type in a collection, in one status and completion, go to
-- another status - null for none. Each spends a version, so a device's write against the old one
-- is told; completion is never touched (UC-FLW-05 check 6).
UPDATE work_item wi SET
  status     = sqlc.narg('to_status')::text,
  updated_at = sqlc.arg('updated_at'),
  version    = wi.version + 1
WHERE wi.tenant_id = current_tenant_id() AND wi.collection_id = sqlc.arg('collection_id')::uuid
  AND coalesce(wi.entry_type, lower(wi.type::text)) = sqlc.arg('entry_type')::text
  AND wi.status IS NOT DISTINCT FROM sqlc.narg('from_status')::text
  AND wi.is_completed = sqlc.arg('is_completed')::boolean
RETURNING wi.id, wi.version;
