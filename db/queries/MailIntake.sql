-- The mail intake (ADR-0094). The tenant is never a parameter except where no tenant is known yet:
-- row level security bounds every other statement to the tenant of the running transaction.

-- name: FindMailSetting :one
SELECT enabled, name, default_collection_id, sender_level, allowed, blocked,
       sender_routes::text AS sender_routes, opened_by, updated_at
FROM mail_setting
WHERE tenant_id = current_tenant_id();

-- name: UpsertMailSetting :exec
INSERT INTO mail_setting (
  tenant_id, enabled, name, default_collection_id, sender_level, allowed, blocked, sender_routes,
  opened_by, updated_at, updated_by
) VALUES (
  current_tenant_id(), sqlc.arg('enabled'), sqlc.narg('name'), sqlc.narg('default_collection_id'),
  sqlc.arg('sender_level'), sqlc.arg('allowed')::text[], sqlc.arg('blocked')::text[],
  sqlc.arg('sender_routes')::jsonb, sqlc.narg('opened_by'), sqlc.arg('updated_at'),
  sqlc.narg('updated_by')
)
ON CONFLICT (tenant_id) DO UPDATE
SET enabled = EXCLUDED.enabled, name = EXCLUDED.name,
    default_collection_id = EXCLUDED.default_collection_id, sender_level = EXCLUDED.sender_level,
    allowed = EXCLUDED.allowed, blocked = EXCLUDED.blocked, sender_routes = EXCLUDED.sender_routes,
    opened_by = EXCLUDED.opened_by, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by;

-- name: MailWorkspaceNameTaken :one
SELECT mail_workspace_name_taken(sqlc.arg('name')::text, current_tenant_id())::boolean AS taken;

-- name: ResolveMailWorkspace :one
-- Before any tenant is known: the function reads past row level security, and only an enabled row.
SELECT resolve_mail_workspace(sqlc.arg('name')::text)::uuid AS tenant_id;

-- name: MailReceiptExists :one
SELECT EXISTS (SELECT 1 FROM mail_receipt WHERE message_id = sqlc.arg('message_id'))::boolean AS seen;

-- name: InsertMailReceipt :execrows
-- A message received twice at once keeps the first receipt; the second inserts nothing.
INSERT INTO mail_receipt (
  id, tenant_id, message_id, received_at, sender, subject, outcome, reason, item_id, entry_id, door
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('message_id'), sqlc.arg('received_at'),
  sqlc.arg('sender'), sqlc.arg('subject'), sqlc.arg('outcome'), sqlc.arg('reason'),
  sqlc.narg('item_id'), sqlc.narg('entry_id'), sqlc.arg('door')
)
ON CONFLICT (tenant_id, message_id) DO NOTHING;

-- name: CountMailReceiptsFrom :one
-- How many messages one sender sent since an instant: the intake's hourly bound per sender.
SELECT count(*) FROM mail_receipt
WHERE sender = sqlc.arg('sender') AND received_at >= sqlc.arg('since');

-- name: RecentMailReceipts :many
SELECT id, message_id, received_at, sender, subject, outcome, reason, item_id, entry_id, door
FROM mail_receipt
ORDER BY received_at DESC, id DESC
LIMIT sqlc.arg('max_rows');

-- name: DeleteExpiredMailReceipts :execrows
-- The retention run's batch: the oldest receipts past the period (ADR-0094, 30 days).
DELETE FROM mail_receipt
WHERE id IN (
  SELECT due.id FROM mail_receipt AS due
  WHERE due.received_at < sqlc.arg('cutoff')
  ORDER BY due.received_at
  LIMIT sqlc.arg('batch')
);

-- name: CountExpiredMailReceipts :one
SELECT count(*) FROM (
  SELECT 1 FROM mail_receipt WHERE received_at < sqlc.arg('cutoff') LIMIT sqlc.arg('ceiling')
) AS due;

-- name: CollectionByHandle :one
-- A live collection in a live hub: a trashed or archived one takes no mail, and the message goes
-- to the jumble saying so.
SELECT c.id
FROM container c
JOIN container hub ON hub.id = c.parent_id
WHERE c.handle = sqlc.arg('handle') AND c.type = 'COLLECTION'
  AND c.deleted_at IS NULL AND c.archived_at IS NULL
  AND hub.deleted_at IS NULL AND hub.archived_at IS NULL;

-- name: ItemByKey :one
SELECT wi.id
FROM work_item wi
JOIN container c ON c.id = wi.collection_id
WHERE c.handle = sqlc.arg('handle') AND wi.number = sqlc.arg('number')::bigint
  AND wi.deleted_at IS NULL AND c.deleted_at IS NULL;

-- name: SetCollectionHandle :execrows
UPDATE container SET handle = sqlc.arg('handle')
WHERE id = sqlc.arg('id') AND type = 'COLLECTION';

-- name: ItemKey :one
-- An entry's ticket key, for the Reply-To of a notification about it.
SELECT c.handle, wi.number
FROM work_item wi
JOIN container c ON c.id = wi.collection_id
WHERE wi.id = sqlc.arg('id') AND wi.deleted_at IS NULL;

-- name: ListMailboxes :many
SELECT id, protocol, address, host, port, session_url, username, folder, done_folder, preset, enabled,
       checked_at, failure, failing_since, created_at
FROM mailbox
ORDER BY created_at, id;

-- name: FindMailbox :one
SELECT id, protocol, address, host, port, session_url, username, folder, done_folder, preset, enabled,
       checked_at, failure, failing_since, created_at, secret_enc, secret_key_id
FROM mailbox
WHERE id = sqlc.arg('id');

-- name: InsertMailbox :exec
INSERT INTO mailbox (
  id, tenant_id, protocol, address, host, port, session_url, username, folder, done_folder, preset,
  enabled, secret_enc, secret_key_id, created_at, created_by
) VALUES (
  sqlc.arg('id'), current_tenant_id(), sqlc.arg('protocol'), sqlc.arg('address'), sqlc.narg('host'),
  sqlc.narg('port'), sqlc.narg('session_url'), sqlc.arg('username'), sqlc.arg('folder'),
  sqlc.arg('done_folder'), sqlc.narg('preset'), sqlc.arg('enabled'), sqlc.arg('secret_enc'),
  sqlc.arg('secret_key_id'), sqlc.arg('created_at'), sqlc.narg('created_by')
);

-- name: SetMailboxChecked :execrows
UPDATE mailbox
SET checked_at = sqlc.arg('checked_at'), failure = sqlc.narg('failure'), failing_since = sqlc.narg('failing_since')
WHERE id = sqlc.arg('id');

-- name: DeleteMailbox :execrows
DELETE FROM mailbox WHERE id = sqlc.arg('id');

-- name: CountMailboxes :one
SELECT count(*) FROM mailbox;
