-- The discussion beside the entries (C-03, domain-model.md §3.5).
--
-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against (ADR-0010).
--
-- The body is stored Unicode NFC normalised, in the database rather than in the application, for
-- the reason container names are: two spellings of the same word are one text to a person, and
-- the domain may not import a Unicode library (ADR-0001, I-W7).

-- name: InsertComment :exec
INSERT INTO comment (
  id, tenant_id, item_id, author_id, parent_comment_id, body, created_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('author_id'),
  sqlc.narg('parent_comment_id'), normalize(sqlc.arg('body')::text, NFC),
  sqlc.arg('created_at'), 1
);

-- name: FindComment :one
-- Tombstones are returned rather than filtered out: whether a deleted comment may be edited is
-- the domain's question, and a query that hid one would turn "it was deleted" into "it never
-- existed" - which is not what a thread full of replies to it says.
SELECT id, tenant_id, item_id, author_id, parent_comment_id, body,
       created_at, edited_at, deleted_at, version
FROM comment
WHERE id = $1;

-- name: ListComments :many
-- One page of one entry's discussion, oldest first: a conversation reads top down, and a page
-- boundary in the middle of it must not reorder what was already read. Tombstones are in it -
-- that is the point of a soft deletion (§3.5) - and the caller serves them without their body.
--
-- Keyset rather than an offset, like every list in this schema (api-guidelines.md §4). The
-- boundary is the pair (created_at, id): two comments written in the same millisecond are one
-- timestamp, and a cursor on the time alone would skip the second or return the first forever.
-- Served by comment_item_idx, whose leading columns are this ORDER BY.
SELECT id, tenant_id, item_id, author_id, parent_comment_id, body,
       created_at, edited_at, deleted_at, version
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
-- The tombstone: text gone, identity and timestamps kept (C-03's acceptance). edited_at survives
-- deliberately - that the words had been rewritten is part of the thread's history, what they
-- were is not.
UPDATE comment SET
  body       = '',
  deleted_at = sqlc.arg('deleted_at'),
  version    = version + 1
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND deleted_at IS NULL;
