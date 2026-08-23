-- Who an entry is on: its member list (C-01).
--
-- The assignee is a column of `work_item` and is written in Work.sql, where every statement about
-- that row lives. The members are their own table, because a set is not a field: they are joined
-- rather than stored as an array, which is what makes them filterable (domain-model.md §6).
--
-- The merge tags both sets share are in Structure.sql. `RecordSetElementAdded` and
-- `RecordSetElementRemoved` already take `set_name` as a parameter for exactly this second caller,
-- so an addition here writes the same shape of tag a label does and the OR-set merge is one
-- implementation rather than two (offline-sync.md §4.2, §10).
--
-- The tenant is never a parameter here, exactly as in Work.sql: it comes from the transaction's own
-- context through current_tenant_id(), which is the same value row level security compares against.

-- name: ListItemMembers :many
-- The accounts one entry carries.
--
-- Ordered by identifier rather than by name. A name is display text and this layer has none to sort
-- by (ADR-0011); the order still has to be stable, because a client comparing two reads of the same
-- entry should not see the list rearrange itself.
--
-- No join against `account`. An account is not soft deleted the way a label is - it is disabled or
-- it is gone, and a gone one takes its rows with it through the tenant-scoped foreign key - so
-- there is no second table whose stamp could hide a row here.
SELECT account_id
FROM item_member
WHERE item_id = $1
ORDER BY account_id;

-- name: AddItemMember :exec
-- ON CONFLICT DO NOTHING rather than a check first, for the reason AddItemLabel gives: adding
-- somebody the entry already carries is the state the caller asked for, and two requests arriving
-- together would otherwise both pass a check and one of them fail on the primary key.
INSERT INTO item_member (tenant_id, item_id, account_id)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('account_id'))
ON CONFLICT DO NOTHING;

-- name: RemoveItemMember :execrows
DELETE FROM item_member
WHERE item_id = sqlc.arg('item_id')::uuid AND account_id = sqlc.arg('account_id')::uuid;

-- How what is created here gets handed out: the assignment policy per scope (C-02).
--
-- One row per scope, which migration 0011's unique index insists on: the row is the storage of
-- the `autoAssign` key of the container's policies document, and a document key cannot be two
-- rows. `state` is ROUND_ROBIN's cursor, kept in the row rather than in the document so that the
-- transaction advancing it can lock exactly what it advances (domain-model.md §3.6).

-- name: FindAutoAssignPolicy :one
SELECT id, tenant_id, scope_type, scope_id, strategy, candidates, state, enabled, version
FROM auto_assign_policy
WHERE scope_type = sqlc.arg('scope_type') AND scope_id = sqlc.arg('scope_id')::uuid;

-- name: LockAutoAssignPolicy :one
-- The same row, held for the rest of the transaction. ROUND_ROBIN reads its cursor through this
-- rather than through FindAutoAssignPolicy: two creates arriving together must queue on the row,
-- because a cursor read hopefully is a turn handed to both of them (C-02's acceptance).
SELECT id, tenant_id, scope_type, scope_id, strategy, candidates, state, enabled, version
FROM auto_assign_policy
WHERE scope_type = sqlc.arg('scope_type') AND scope_id = sqlc.arg('scope_id')::uuid
FOR UPDATE;

-- name: UpsertAutoAssignPolicy :exec
-- The whole definition in one statement, because the caller holds PUT semantics: the policies
-- document arrives complete, so the row it maps to is written complete. A rewrite resets the
-- state - the rotation belongs to the pool that was configured, and a new pool starts at its
-- head rather than at an index into a list that no longer exists.
INSERT INTO auto_assign_policy
  (id, tenant_id, scope_type, scope_id, strategy, candidates, state, enabled, version)
VALUES (
  sqlc.arg('id')::uuid, current_tenant_id(), sqlc.arg('scope_type'), sqlc.arg('scope_id')::uuid,
  sqlc.arg('strategy'), sqlc.arg('candidates'), '{}'::jsonb, sqlc.arg('enabled'), 1
)
ON CONFLICT (tenant_id, scope_type, scope_id) DO UPDATE SET
  strategy   = EXCLUDED.strategy,
  candidates = EXCLUDED.candidates,
  state      = '{}'::jsonb,
  enabled    = EXCLUDED.enabled,
  version    = auto_assign_policy.version + 1;

-- name: DeleteAutoAssignPolicy :execrows
-- Removing the key from the document removes the row. Idempotent at the caller: a document that
-- never carried the key deletes nothing, and that is the state that was asked for.
DELETE FROM auto_assign_policy
WHERE scope_type = sqlc.arg('scope_type') AND scope_id = sqlc.arg('scope_id')::uuid;

-- name: SaveAutoAssignPolicyState :execrows
-- The advanced cursor, written by the transaction that holds the lock LockAutoAssignPolicy took.
-- No version guard: the lock is the concurrency control here, and a version bump per assignment
-- would make the rotation's bookkeeping look like a configuration change to a client comparing
-- versions.
UPDATE auto_assign_policy
SET state = sqlc.arg('state')
WHERE id = sqlc.arg('id')::uuid;
