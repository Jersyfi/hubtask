-- The jumble (G-10, domain-model.md §2): entries arrive, are decided about exactly once, and the
-- dismissed ones age out by retention rule.
--
-- The tenant is never a parameter: row level security bounds every statement to the tenant of the
-- running transaction (ADR-0010, multi-tenancy.md §2).

-- name: InsertJumbleEntry :exec
INSERT INTO jumble_entry (
  id, tenant_id, channel, sender, raw_subject, raw_body, attachments, status, received_at
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('channel'), sqlc.narg('sender'),
  sqlc.narg('raw_subject'), sqlc.narg('raw_body'), sqlc.arg('attachments'),
  sqlc.arg('status'), sqlc.arg('received_at')
);

-- name: FindJumbleEntry :one
SELECT id, channel, sender, raw_subject, raw_body, attachments, suggestion, status,
       target_item_id, received_at, processed_at
FROM jumble_entry
WHERE id = sqlc.arg('id');

-- name: ListJumbleEntries :many
-- Newest first by identifier: UUIDv7 is time-ordered, so the primary key is the arrival order.
-- The two filters are nullable arguments rather than four statements, for the run log's reason: a
-- second statement differing in one predicate is a second place for a predicate to be forgotten.
SELECT id, channel, sender, raw_subject, raw_body, attachments, suggestion, status,
       target_item_id, received_at, processed_at
FROM jumble_entry
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('channel')::text IS NULL OR channel = sqlc.narg('channel')::text)
  AND (sqlc.narg('after')::uuid IS NULL OR id < sqlc.narg('after')::uuid)
ORDER BY id DESC
LIMIT sqlc.arg('page_size');

-- name: SettleJumbleEntry :execrows
-- The one statement that decides an entry, whichever way it was decided. The status guard is in
-- the WHERE rather than in a read-then-write: two conversions racing both read NEW, and the
-- second would produce a second item - here the second matches nothing, and zero rows is what
-- "somebody got there first" looks like from a statement that cannot be raced.
UPDATE jumble_entry
SET status         = sqlc.arg('status'),
    target_item_id = sqlc.narg('target_item_id'),
    processed_at   = sqlc.arg('settled_at')
WHERE id = sqlc.arg('id') AND status = 'NEW';
