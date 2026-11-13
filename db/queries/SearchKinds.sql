-- The search over every kind besides entries and pages (UC-WRK-18 check 9): each query
-- answers the candidates whose name holds every one of the words, lowered, best first (a name
-- that begins with the first word, then the last changed), at most `batch` of them. Which of them
-- the reader may read is the use case's question (rule 2), so every row names its place. The
-- words are bound as an array and compared with strpos, so no byte of them becomes SQL text or a
-- pattern (rule 9).

-- name: SearchWhiteboardNames :many
SELECT w.id, w.hub_id, w.title, w.updated_at,
       (strpos(lower(w.title), (sqlc.arg('terms')::text[])[1]) = 1) AS leads
FROM whiteboard w
WHERE w.tenant_id = current_tenant_id() AND w.deleted_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR w.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(w.title), t) = 0)
ORDER BY leads DESC, w.updated_at DESC, w.id
LIMIT sqlc.arg('batch');

-- name: SearchGoalNames :many
SELECT g.id, g.hub_id, g.title, g.status, g.updated_at,
       (strpos(lower(g.title), (sqlc.arg('terms')::text[])[1]) = 1) AS leads
FROM goal g
WHERE g.tenant_id = current_tenant_id()
  AND (sqlc.narg('hub_id')::uuid IS NULL OR g.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(g.title), t) = 0)
ORDER BY leads DESC, g.updated_at DESC, g.id
LIMIT sqlc.arg('batch');

-- name: SearchDashboardNames :many
SELECT d.id, d.hub_id, d.owner_id, d.name, d.sharing, d.group_ids, d.updated_at,
       (strpos(lower(d.name), (sqlc.arg('terms')::text[])[1]) = 1) AS leads
FROM dashboard d
WHERE d.tenant_id = current_tenant_id()
  AND (sqlc.narg('hub_id')::uuid IS NULL OR d.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(d.name), t) = 0)
ORDER BY leads DESC, d.updated_at DESC, d.id
LIMIT sqlc.arg('batch');

-- name: SearchCiNames :many
-- A merged item lives on in the one it was merged into.
SELECT c.id, c.hub_id, c.class_key, c.name, c.stage, c.updated_at,
       (strpos(lower(c.name), (sqlc.arg('terms')::text[])[1]) = 1) AS leads
FROM cmdb_ci c
WHERE c.tenant_id = current_tenant_id() AND c.merged_into IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(c.name), t) = 0)
ORDER BY leads DESC, c.updated_at DESC, c.id
LIMIT sqlc.arg('batch');

-- name: SearchPeopleNames :many
-- People by their display name alone - never their address, which another member does not see
-- (data-protection.md §9); service accounts and erased people are not people to find.
SELECT a.id, a.display_name, a.kind::text AS kind, a.status::text AS status, a.updated_at,
       (strpos(lower(a.display_name), (sqlc.arg('terms')::text[])[1]) = 1) AS leads
FROM account a
WHERE a.tenant_id = current_tenant_id() AND a.deleted_at IS NULL AND a.kind = 'USER'
  AND a.status <> 'ANONYMIZED'
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(a.display_name), t) = 0)
ORDER BY leads DESC, a.display_name COLLATE hubtask_name, a.id
LIMIT sqlc.arg('batch');

-- name: SearchEntryFileNames :many
-- The files on living entries by their names; a file's contents are not searched.
SELECT m.id, m.file_name::text AS file_name, m.mime_type, m.created_at, i.id AS item_id,
       i.collection_id, c.parent_id AS hub_id,
       (strpos(lower(m.file_name), (sqlc.arg('terms')::text[])[1]) = 1) AS leads
FROM item_attachment a
JOIN media_object m ON m.tenant_id = a.tenant_id AND m.id = a.media_id
JOIN work_item i ON i.tenant_id = a.tenant_id AND i.id = a.item_id
JOIN container c ON c.tenant_id = i.tenant_id AND c.id = i.collection_id
WHERE a.tenant_id = current_tenant_id() AND m.deleted_at IS NULL AND m.file_name IS NOT NULL
  AND i.deleted_at IS NULL AND i.archived_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.parent_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(m.file_name), t) = 0)
ORDER BY leads DESC, m.created_at DESC, m.id
LIMIT sqlc.arg('batch');

-- name: SearchEntryComments :many
-- A person's comments on living entries by their words; the server's own lines are not searched.
SELECT cm.id, cm.body, coalesce(cm.edited_at, cm.created_at)::timestamptz AS changed_at, i.id AS item_id,
       i.title AS item_title, i.collection_id, c.parent_id AS hub_id
FROM comment cm
JOIN work_item i ON i.tenant_id = cm.tenant_id AND i.id = cm.item_id
JOIN container c ON c.tenant_id = i.tenant_id AND c.id = i.collection_id
WHERE cm.tenant_id = current_tenant_id() AND cm.deleted_at IS NULL AND cm.kind = 'USER'
  AND i.deleted_at IS NULL AND i.archived_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.parent_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(cm.body), t) = 0)
ORDER BY changed_at DESC, cm.id
LIMIT sqlc.arg('batch');
