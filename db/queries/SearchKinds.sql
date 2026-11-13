-- The search over every kind besides entries and pages (UC-WRK-18 check 9): each query
-- answers the candidates whose name holds every one of the words, lowered, best first (a name
-- that begins with the first word, then the last changed), at most `batch` of them. Which of them
-- the reader may read is the use case's question (rule 2), so every row names its place. The
-- words are bound as an array and compared with strpos, so no byte of them becomes SQL text or a
-- pattern (rule 9); no words is every name.
--
-- A key of one kind (asset.class, goal.state, ...) is three bound parameters: `<key>_negated`
-- (null where the key is not asked), `<key>_values` (the column is one of them) and `<key>_none`
-- (or the column is empty); the match is turned round where negated.

-- name: SearchWhiteboardNames :many
SELECT w.id, w.hub_id, w.title, w.updated_at,
       coalesce(strpos(lower(w.title), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
FROM whiteboard w
WHERE w.tenant_id = current_tenant_id() AND w.deleted_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR w.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(w.title), t) = 0)
ORDER BY leads DESC, w.updated_at DESC, w.id
LIMIT sqlc.arg('batch');

-- name: SearchGoalNames :many
SELECT g.id, g.hub_id, g.title, g.status, g.updated_at,
       coalesce(strpos(lower(g.title), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
FROM goal g
WHERE g.tenant_id = current_tenant_id()
  AND (sqlc.narg('hub_id')::uuid IS NULL OR g.hub_id = sqlc.narg('hub_id')::uuid)
  AND (sqlc.narg('state_negated')::boolean IS NULL
       OR coalesce(g.status = ANY(sqlc.arg('state_values')::text[]), false) <> sqlc.narg('state_negated')::boolean)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(g.title), t) = 0)
ORDER BY leads DESC, g.updated_at DESC, g.id
LIMIT sqlc.arg('batch');

-- name: SearchDashboardNames :many
SELECT d.id, d.hub_id, d.owner_id, d.name, d.sharing, d.group_ids, d.updated_at,
       coalesce(strpos(lower(d.name), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
FROM dashboard d
WHERE d.tenant_id = current_tenant_id()
  AND (sqlc.narg('hub_id')::uuid IS NULL OR d.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(d.name), t) = 0)
ORDER BY leads DESC, d.updated_at DESC, d.id
LIMIT sqlc.arg('batch');

-- name: SearchCiNames :many
-- A merged item lives on in the one it was merged into. Its owner is a person or a group; its
-- tag is the asset record's, empty where none was written.
SELECT c.id, c.hub_id, c.class_key, c.name, c.stage, c.updated_at,
       coalesce(strpos(lower(c.name), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
FROM cmdb_ci c
LEFT JOIN cmdb_asset a ON a.tenant_id = c.tenant_id AND a.ci_id = c.id
WHERE c.tenant_id = current_tenant_id() AND c.merged_into IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.hub_id = sqlc.narg('hub_id')::uuid)
  AND (sqlc.narg('class_negated')::boolean IS NULL
       OR coalesce(lower(c.class_key) = ANY(sqlc.arg('class_values')::text[]), false) <> sqlc.narg('class_negated')::boolean)
  AND (sqlc.narg('stage_negated')::boolean IS NULL
       OR coalesce(c.stage = ANY(sqlc.arg('stage_values')::text[]), false) <> sqlc.narg('stage_negated')::boolean)
  AND (sqlc.narg('owner_negated')::boolean IS NULL
       OR (coalesce(c.owner_account_id = ANY(sqlc.arg('owner_values')::uuid[])
                    OR c.owner_group_id = ANY(sqlc.arg('owner_values')::uuid[]), false)
           OR (sqlc.arg('owner_none')::boolean AND c.owner_account_id IS NULL AND c.owner_group_id IS NULL))
          <> sqlc.narg('owner_negated')::boolean)
  AND (sqlc.narg('location_negated')::boolean IS NULL
       OR (coalesce(c.location_ci_id = ANY(sqlc.arg('location_values')::uuid[]), false)
           OR (sqlc.arg('location_none')::boolean AND c.location_ci_id IS NULL))
          <> sqlc.narg('location_negated')::boolean)
  AND (sqlc.narg('tag_negated')::boolean IS NULL
       OR (coalesce(lower(nullif(a.asset_tag, '')) = ANY(sqlc.arg('tag_values')::text[]), false)
           OR (sqlc.arg('tag_none')::boolean AND nullif(a.asset_tag, '') IS NULL))
          <> sqlc.narg('tag_negated')::boolean)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(c.name), t) = 0)
ORDER BY leads DESC, c.updated_at DESC, c.id
LIMIT sqlc.arg('batch');

-- name: SearchPeopleNames :many
-- People by their display name alone - never their address, which another member does not see
-- (data-protection.md §9); service accounts and erased people are not people to find.
SELECT a.id, a.display_name, a.kind::text AS kind, a.status::text AS status, a.updated_at,
       coalesce(strpos(lower(a.display_name), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
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
       coalesce(strpos(lower(m.file_name), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
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

-- name: SearchPageFileNames :many
-- The files on living pages by their names; a file's contents are not searched.
SELECT m.id, m.file_name::text AS file_name, m.mime_type, m.created_at, p.id AS page_id, p.hub_id,
       coalesce(strpos(lower(m.file_name), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
FROM page_media pm
JOIN media_object m ON m.tenant_id = pm.tenant_id AND m.id = pm.media_id
JOIN page p ON p.tenant_id = pm.tenant_id AND p.id = pm.page_id
WHERE pm.tenant_id = current_tenant_id() AND m.deleted_at IS NULL AND m.file_name IS NOT NULL
  AND p.deleted_at IS NULL AND p.archived_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR p.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(m.file_name), t) = 0)
ORDER BY leads DESC, m.created_at DESC, m.id
LIMIT sqlc.arg('batch');

-- name: SearchPageComments :many
-- The comments on living pages by their words; a comment taken back is not searched.
SELECT pc.id, pc.body, coalesce(pc.edited_at, pc.created_at)::timestamptz AS changed_at, p.id AS page_id,
       p.title AS page_title, p.hub_id
FROM page_comment pc
JOIN page p ON p.tenant_id = pc.tenant_id AND p.id = pc.page_id
WHERE pc.tenant_id = current_tenant_id() AND pc.deleted_at IS NULL
  AND p.deleted_at IS NULL AND p.archived_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR p.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(pc.body), t) = 0)
ORDER BY changed_at DESC, pc.id
LIMIT sqlc.arg('batch');

-- name: SearchWhiteboardFileNames :many
-- The files on living whiteboards by their names; a file's contents are not searched.
SELECT m.id, m.file_name::text AS file_name, m.mime_type, m.created_at, w.id AS whiteboard_id, w.hub_id,
       coalesce(strpos(lower(m.file_name), (sqlc.arg('terms')::text[])[1]) = 1, false) AS leads
FROM whiteboard_media wm
JOIN media_object m ON m.tenant_id = wm.tenant_id AND m.id = wm.media_id
JOIN whiteboard w ON w.tenant_id = wm.tenant_id AND w.id = wm.whiteboard_id
WHERE wm.tenant_id = current_tenant_id() AND m.deleted_at IS NULL AND m.file_name IS NOT NULL
  AND w.deleted_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR w.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(m.file_name), t) = 0)
ORDER BY leads DESC, m.created_at DESC, m.id
LIMIT sqlc.arg('batch');

-- name: SearchWhiteboardComments :many
-- The comments on living whiteboards by their words; a comment taken back is not searched.
SELECT wc.id, wc.body, coalesce(wc.edited_at, wc.created_at)::timestamptz AS changed_at, w.id AS whiteboard_id,
       w.title AS whiteboard_title, w.hub_id
FROM whiteboard_comment wc
JOIN whiteboard w ON w.tenant_id = wc.tenant_id AND w.id = wc.whiteboard_id
WHERE wc.tenant_id = current_tenant_id() AND wc.deleted_at IS NULL AND w.deleted_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR w.hub_id = sqlc.narg('hub_id')::uuid)
  AND NOT EXISTS (SELECT 1 FROM unnest(sqlc.arg('terms')::text[]) t WHERE strpos(lower(wc.body), t) = 0)
ORDER BY changed_at DESC, wc.id
LIMIT sqlc.arg('batch');

-- name: SearchEntryByKey :many
-- The living entry a ticket key names (UC-WRK-18 check 10): the collection by its key prefix or
-- its handle, as the mail intake reads a key (ItemByKey), the prefix preferred; at most one.
SELECT i.id, i.title, i.type::text AS level, i.is_completed, i.updated_at, i.number,
       i.collection_id, c.parent_id AS hub_id,
       coalesce(c.key_prefix, upper(c.handle))::text AS key_word
FROM work_item i
JOIN container c ON c.tenant_id = i.tenant_id AND c.id = i.collection_id
WHERE i.tenant_id = current_tenant_id()
  AND (c.handle = lower(sqlc.arg('word')::text) OR c.key_prefix = upper(sqlc.arg('word')::text))
  AND i.number = sqlc.arg('number')::bigint
  AND i.deleted_at IS NULL AND i.archived_at IS NULL AND c.deleted_at IS NULL
  AND (sqlc.narg('hub_id')::uuid IS NULL OR c.parent_id = sqlc.narg('hub_id')::uuid)
ORDER BY (c.key_prefix IS NOT NULL AND c.key_prefix = upper(sqlc.arg('word')::text)) DESC
LIMIT 1;
