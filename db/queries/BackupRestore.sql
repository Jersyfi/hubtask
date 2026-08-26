-- The statements a restore reads and records itself through (E-06, backup-restore.md §7, §8).
--
-- Row level security supplies the tenant condition none of these statements writes (ADR-0010),
-- which is BK-10 at the layer where it cannot be forgotten: a restore into another tenant's
-- archive is refused by the use case, and a statement that reached across anyway would find
-- nothing.

-- name: JournalledDeletions :many
-- The deletion journal, read for the first time in production (§7).
--
-- Paged on (deleted_at, entity, entity_id) rather than on OFFSET, for the reason the export is:
-- a page over a set another statement may reorder can repeat a row or drop one, and dropping one
-- here means an object comes back that somebody asked to have deleted.
SELECT entity, entity_id, deleted_at, reason
FROM deletion_journal
WHERE deleted_at > sqlc.arg('since')::timestamptz
  AND (deleted_at, entity, entity_id)
      > (sqlc.arg('after_at')::timestamptz, sqlc.arg('after_entity')::text, sqlc.arg('after_id')::uuid)
ORDER BY deleted_at, entity, entity_id
LIMIT sqlc.arg('batch')::int;
