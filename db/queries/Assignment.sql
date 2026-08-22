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
