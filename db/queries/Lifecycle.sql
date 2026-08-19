-- The end of data's life: the instructions not to delete, and the record of what was deleted anyway
-- (B-10, ADR-0020).
--
-- The tenant is never a parameter here either: it comes from the transaction's own context through
-- current_tenant_id(), which is the value row level security compares against. That matters more
-- here than anywhere else - a legal hold read across the wrong boundary would not be a wrong answer
-- but somebody else's obligation ignored.

-- name: ActiveLegalHolds :many
-- Every hold in force for this tenant, released ones left out.
--
-- The whole set rather than a question per row: a purge run judges a batch of a thousand rows
-- against holds that are few and change rarely, and a query per row would be a thousand round trips
-- for the same answer. Served by legal_hold_active_idx, which is partial on exactly this predicate.
SELECT id, scope_kind, scope_id, reason, placed_at
FROM legal_hold
WHERE released_at IS NULL
ORDER BY placed_at;

-- name: RecordDeletions :exec
-- The journal: what was removed, when, and why.
--
-- Its purpose is a restore from backup. An archive taken before the deletion still holds the row,
-- and without this the next restore would quietly bring back everything a tenant had deleted since
-- (backup-restore.md §6) - so the journal outlives both the row and the backup.
--
-- Written from an array rather than row by row, because a retention run removes in batches of a
-- thousand and a statement per row would make the transaction as long as the batch. One entity per
-- call, so the array is identifiers alone: a purge that spans both tables makes two calls, which is
-- cheaper than carrying the table name a thousand times to say the same thing.
--
-- ON CONFLICT DO NOTHING, because the whole job is at-least-once: a run that died after writing
-- these and is picked up again writes the same rows, and a failure there would make the retry
-- impossible rather than harmless.
INSERT INTO deletion_journal (tenant_id, entity, entity_id, deleted_at, reason)
SELECT
  current_tenant_id(), sqlc.arg('entity')::text, entity_id,
  sqlc.arg('deleted_at')::timestamptz, sqlc.arg('reason')::text
FROM unnest(sqlc.arg('entity_ids')::uuid[]) AS entity_id
ON CONFLICT (tenant_id, entity, entity_id) DO NOTHING;

-- name: RecordTombstones :exec
-- The markers that stop a device recreating what it still knows (offline-sync.md §7).
--
-- Without one, the classic bug appears: a device offline for eight weeks pushes a change for a row
-- the server has no record of, and the server accepts it. `purge_after` is when the marker itself
-- may go - the removal plus the maximum offline window, by which time a device with a cursor that
-- old has to resynchronise from scratch anyway.
--
-- One array per entity and ON CONFLICT DO NOTHING for the reasons the journal has them.
INSERT INTO tombstone (tenant_id, entity, entity_id, deleted_at, purge_after)
SELECT
  current_tenant_id(), sqlc.arg('entity')::text, entity_id,
  sqlc.arg('deleted_at')::timestamptz, sqlc.arg('purge_after')::timestamptz
FROM unnest(sqlc.arg('entity_ids')::uuid[]) AS entity_id
ON CONFLICT (tenant_id, entity, entity_id) DO NOTHING;
