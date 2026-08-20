-- The item history: what happened to a piece of work, in the words the people working on it read
-- (B-11, domain-model.md §3.5).
--
-- Append-only. There is deliberately no UPDATE and no DELETE in this file: an entry is not edited,
-- and what removes one is the deletion of the item it belongs to, through the foreign key
-- (0009_activity_history.sql). A statement that deleted one by hand would be a history somebody
-- could tidy up, which is the opposite of what a history is for.
--
-- The tenant is never a parameter. It comes from the transaction's own context through
-- current_tenant_id(), which is the value row level security compares against (ADR-0010).

-- name: RecordActivity :exec
-- One step of one item's history, written inside the transaction that made the change.
--
-- `container_id` is the collection the item was in when this happened - the visibility anchor
-- rather than a second subject. It is stored rather than joined at read time because the item may
-- have moved since, and a history that re-derived it would show where the entry is now instead of
-- where it was then.
--
-- The correlation and causation columns stay NULL. They belong to an act caused by another act,
-- and everything this milestone records is something a person asked for directly; filling them
-- with a request identifier would be a chain that describes nothing (automation.md §2).
INSERT INTO activity_entry (
  id, tenant_id, item_id, container_id, actor_type, actor_id, verb, change_set, occurred_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.narg('container_id'),
  sqlc.arg('actor_type'), sqlc.narg('actor_id'), sqlc.arg('verb'),
  sqlc.arg('change_set'), sqlc.arg('occurred_at')
);

-- name: ListActivity :many
-- One page of an item's history, newest first.
--
-- Keyset rather than an offset, like every other list in this schema (api-guidelines.md §4): an
-- offset re-reads everything it skips and shifts under a concurrent write, and a history is written
-- to precisely while somebody is reading it. The boundary is the pair (occurred_at, id), because
-- two steps of one act share a timestamp - a cursor on the time alone would either skip the second
-- or return the first again forever.
--
-- Served by activity_page_idx, whose columns are this ORDER BY.
SELECT id, item_id, container_id, actor_type, actor_id, verb, change_set, occurred_at
FROM activity_entry
WHERE item_id = sqlc.arg('item_id')
  AND (
    sqlc.narg('cursor_occurred_at')::timestamptz IS NULL
    OR (occurred_at, id) < (sqlc.narg('cursor_occurred_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY occurred_at DESC, id DESC
LIMIT sqlc.arg('page_size');
