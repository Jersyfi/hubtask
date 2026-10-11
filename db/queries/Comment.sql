-- The discussion beside the entries (domain-model.md §3.5).
--
-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against (ADR-0010).
--
-- The body arrives in Unicode normal form C, for the reason a container's name does
-- (InsertContainer): the constructor brings it there, and normalize() stays on the insert
-- and the edit as the row's own guarantee for a writer that reaches it without the constructor.

-- name: InsertComment :exec
INSERT INTO comment (
  id, tenant_id, item_id, author_id, parent_comment_id, body, created_at, version,
  kind, system_code, system_params
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('author_id'),
  sqlc.narg('parent_comment_id'), normalize(sqlc.arg('body')::text, NFC),
  sqlc.arg('created_at'), 1,
  sqlc.arg('kind'), sqlc.narg('system_code'), sqlc.narg('system_params')
);

-- name: FindComment :one
-- Tombstones are returned rather than filtered out: whether a deleted comment may be edited is
-- the domain's question, and a query that hid one would turn "it was deleted" into "it never
-- existed" - which is not what a thread full of replies to it says.
--
-- A tombstone's body is answered empty whatever is stored: the text a legal hold keeps
-- (SetCommentDeleted) is the hold's, never a reader's, and masking it here is what keeps every door
-- from serving it.
SELECT id, tenant_id, item_id, author_id, parent_comment_id,
       (CASE WHEN deleted_at IS NULL THEN body ELSE '' END)::text AS body,
       created_at, edited_at, deleted_at, version, kind, system_code, system_params
FROM comment
WHERE id = $1;

-- name: ListComments :many
-- One page of one entry's discussion, oldest first: a conversation reads top down, and a page
-- boundary in the middle of it must not reorder what was already read. Tombstones are in it -
-- that is the point of a soft deletion (§3.5) - and the caller serves them without their body,
-- which is answered empty here whatever a hold keeps (FindComment).
--
-- Keyset rather than an offset, like every list in this schema (api-guidelines.md §4). The
-- boundary is the pair (created_at, id): two comments written in the same millisecond are one
-- timestamp, and a cursor on the time alone would skip the second or return the first forever.
-- Served by comment_item_idx, whose leading columns are this ORDER BY.
SELECT id, tenant_id, item_id, author_id, parent_comment_id,
       (CASE WHEN deleted_at IS NULL THEN body ELSE '' END)::text AS body,
       created_at, edited_at, deleted_at, version, kind, system_code, system_params
FROM comment
WHERE item_id = sqlc.arg('item_id')
  AND (
    sqlc.narg('cursor_created_at')::timestamptz IS NULL
    OR (created_at, id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at, id
LIMIT sqlc.arg('page_size');

-- name: SetCommentBody :execrows
-- The rewrite, under the same optimistic lock every other row takes (api-guidelines.md §5). The
-- deletion stamp is in the guard rather than trusted to the caller's read: a tombstone's text is
-- gone, and an edit racing a deletion must lose to it rather than resurrect the words.
UPDATE comment SET
  body      = normalize(sqlc.arg('body')::text, NFC),
  edited_at = sqlc.arg('edited_at'),
  version   = version + 1
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND deleted_at IS NULL;

-- name: SetCommentDeleted :execrows
-- The tombstone: text gone, identity and timestamps kept. edited_at survives
-- deliberately - that the words had been rewritten is part of the thread's history, what they
-- were is not.
--
-- keep_text is a legal hold covering the comment (data-retention.md §4): the row is a tombstone for
-- every reader all the same, and the text stays until the retention pass clears it
-- (ClearCommentTexts) once no hold covers it.
UPDATE comment SET
  body       = CASE WHEN sqlc.arg('keep_text')::boolean THEN body ELSE '' END,
  deleted_at = sqlc.arg('deleted_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND deleted_at IS NULL;

-- name: KeptCommentTexts :many
-- The tombstones whose text a legal hold kept, with where their entry is: what a hold is judged
-- against (lifecycle.Target) - the entry's path, its collection and the collection's hub. One page
-- in identifier order; the pass walks every page, because a text still held stays in the set.
-- Served by comment_kept_text_idx, which is partial on exactly this predicate.
SELECT c.id, c.item_id, w.path, w.collection_id, col.parent_id AS hub_id
FROM comment c
JOIN work_item w ON w.tenant_id = c.tenant_id AND w.id = c.item_id
JOIN container col ON col.tenant_id = w.tenant_id AND col.id = w.collection_id
WHERE c.tenant_id = current_tenant_id() AND c.deleted_at IS NOT NULL AND c.body <> ''
  AND c.id > sqlc.arg('after')::uuid
ORDER BY c.id
LIMIT sqlc.arg('batch');

-- name: ClearCommentTexts :execrows
-- The kept text, gone once no hold covers it. Nothing else of the row changes and no version is
-- spent: every reader already saw an empty body (FindComment), so nobody can tell the difference
-- and nothing is announced. A living comment is never matched.
UPDATE comment SET body = ''
WHERE tenant_id = current_tenant_id() AND deleted_at IS NOT NULL AND body <> ''
  AND id = ANY(sqlc.arg('ids')::uuid[]);
