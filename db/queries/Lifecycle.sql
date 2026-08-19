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

-- name: ExpiredTrashItems :many
-- The entries whose time in the trash is up, deepest first.
--
-- Deepest first because a purge works a subtree from the bottom up (data-retention.md §4.6): a
-- parent removed before its children would take them with it through the foreign key, and rows
-- removed by a cascade nobody counted are exactly the orphans that leave no journal entry and no
-- tombstone behind (ADR-0020 §6).
--
-- The cutoff is a parameter rather than a period, so that the caller reads the clock once for the
-- whole run - two readings would let a long batch use two different definitions of "expired". A
-- cutoff of now is what emptying a trash by hand passes.
--
-- `hub_id` travels because a legal hold placed on a hub has to reach an entry three levels below it,
-- and the entry alone does not know which hub it is under. The join is a primary key lookup.
--
-- The limit is the batch: a retention run removes in batches so that a large deletion does not hold
-- one transaction open across the whole of it (data-retention.md §5).
SELECT i.id, i.type, i.path, i.collection_id, col.parent_id AS hub_id, i.deleted_at
FROM work_item i
JOIN container col ON col.id = i.collection_id
WHERE i.deleted_at IS NOT NULL
  AND i.deleted_at < sqlc.arg('cutoff')::timestamptz
ORDER BY i.depth DESC, i.deleted_at, i.id
LIMIT sqlc.arg('batch_size');

-- name: ExpiredTrashContainers :many
-- The hubs and collections whose time in the trash is up, collections before the hubs that hold them.
--
-- The order is the same bottom-up rule and the same reason: `container.parent_id` is
-- ON DELETE RESTRICT, so a hub whose collections are still there refuses to go - the database
-- insists on the order, and this is where it is obeyed rather than discovered.
SELECT c.id, c.type, c.parent_id, c.deleted_at
FROM container c
WHERE c.deleted_at IS NOT NULL
  AND c.deleted_at < sqlc.arg('cutoff')::timestamptz
ORDER BY (c.type = 'HUB'), c.deleted_at, c.id
LIMIT sqlc.arg('batch_size');

-- name: EnsureRetentionPolicy :exec
-- The default period for one kind, written for a tenant that has none.
--
-- Seeded by the run rather than by a migration. A migration would cover the tenants that existed
-- when it ran and no others, so the first tenant created afterwards would be one with no policy -
-- and the defaults live in code either way, so a second copy of them in SQL would be a second place
-- for them to drift from the document (data-retention.md §3).
--
-- DO NOTHING rather than an upsert: a tenant that has changed its period has decided something, and
-- a sweep that reset it to the default every time it ran would be a sweep that quietly overrode the
-- tenant it is running for.
INSERT INTO retention_policy (tenant_id, data_kind, retain_days, min_days)
VALUES (current_tenant_id(), sqlc.arg('data_kind'), sqlc.arg('retain_days'), sqlc.arg('min_days'))
ON CONFLICT (tenant_id, data_kind) DO NOTHING;

-- name: FindRetentionPolicy :one
-- The period in force for one kind.
SELECT data_kind, retain_days, min_days, max_days
FROM retention_policy
WHERE data_kind = sqlc.arg('data_kind');
