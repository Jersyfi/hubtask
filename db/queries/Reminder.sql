-- The reminders beside the entries (D-02, domain-model.md §3.5).
--
-- The tenant is never a parameter here: it comes from the transaction's own context through
-- current_tenant_id(), which is the same value row level security compares against (ADR-0010).
--
-- Nothing in this file computes a moment. fire_at arrives already decided, because deciding it is
-- reading an offset spec against a due date in an IANA zone - domain arithmetic (rule 4), and the
-- one place it may live is the one that can be tested without a database.

-- name: InsertReminder :exec
INSERT INTO reminder (
  id, tenant_id, item_id, offset_spec, channels, recipients, state, fire_at, created_at, version
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('offset_spec'),
  sqlc.arg('channels')::text[], sqlc.arg('recipients')::uuid[], sqlc.arg('state'),
  sqlc.narg('fire_at'), sqlc.arg('created_at'), 1
);

-- name: FindReminder :one
SELECT id, tenant_id, item_id, offset_spec, channels, recipients, state, fire_at,
       created_at, updated_at, version
FROM reminder
WHERE id = $1;

-- name: ListRemindersOfItem :many
-- One entry's reminders, oldest first, and all of them: the number a single entry may carry is
-- bounded where reminders are written, which is what makes this list answerable in one page rather
-- than through a cursor (D-02). The order is (created_at, id) for the reason every list in this
-- schema takes the pair: two rows written in the same millisecond are one timestamp. Served by
-- reminder_item_idx, whose columns are this ORDER BY.
SELECT id, tenant_id, item_id, offset_spec, channels, recipients, state, fire_at,
       created_at, updated_at, version
FROM reminder
WHERE item_id = sqlc.arg('item_id')
ORDER BY created_at, id;

-- name: ListPendingRemindersOfItem :many
-- What a moved due date has to recompute: the entry's reminders that are still waiting. A reminder
-- that has fired or was cancelled is not in it - neither is given a new future by a date changing
-- (D-02) - and which of the remaining ones actually move is the domain's question, because only it
-- can read an offset spec.
SELECT id, tenant_id, item_id, offset_spec, channels, recipients, state, fire_at,
       created_at, updated_at, version
FROM reminder
WHERE item_id = sqlc.arg('item_id') AND state = 'PENDING'
ORDER BY created_at, id;

-- name: CountRemindersOfItem :one
-- The bound's other half, read in the same transaction as the insert so that two concurrent
-- creations cannot both find room for the last one.
SELECT count(*) FROM reminder WHERE item_id = sqlc.arg('item_id');

-- name: UpdateReminder :execrows
-- The edit, under the same optimistic lock every other row takes (api-guidelines.md §5). The state
-- is in the guard rather than trusted to the caller's read: a reminder that fired while the edit
-- was being written must lose to the firing rather than be given a new moment afterwards.
UPDATE reminder SET
  offset_spec = sqlc.arg('offset_spec'),
  channels    = sqlc.arg('channels')::text[],
  recipients  = sqlc.arg('recipients')::uuid[],
  fire_at     = sqlc.narg('fire_at'),
  updated_at  = sqlc.arg('updated_at'),
  version     = version + 1
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version')
  AND state = 'PENDING';

-- name: SetReminderFireAt :execrows
-- The recomputation a moved due date causes. Neither the version nor the stamp moves with it:
-- fire_at is derived from the offset and the entry's date, so nobody edited this row - and a
-- version spent here would answer a client's If-Match with a conflict it could not have avoided.
-- What serialises two concurrent moves is the entry's own optimistic lock, in whose transaction
-- this runs.
UPDATE reminder SET
  fire_at = sqlc.narg('fire_at')
WHERE id = sqlc.arg('id')::uuid
  AND state = 'PENDING';

-- name: DeleteReminder :execrows
-- A hard delete of one row: a reminder is created and deleted whole, and what somebody deleted is
-- gone rather than kept as a tombstone with a state (D-02). The change log carries the deletion to
-- offline clients, which is where a tombstone belongs (offline-sync.md §7).
DELETE FROM reminder
WHERE id = sqlc.arg('id')::uuid
  AND version = sqlc.arg('expected_version');
