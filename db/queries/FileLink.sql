-- A file in a store on an entry (migration 0276, UC-INT-14).

-- name: ListFileLinks :many
SELECT id, item_id, provider, address, name, added_by, added_at
FROM file_link
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id')
ORDER BY added_at, id;

-- name: CountFileLinks :one
SELECT count(*)::int AS links FROM file_link
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id');

-- name: FindFileLink :one
SELECT id, item_id, provider, address, name, added_by, added_at
FROM file_link
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: InsertFileLink :execrows
INSERT INTO file_link (id, tenant_id, item_id, provider, address, name, added_by, added_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('provider'), sqlc.arg('address'),
        sqlc.arg('name'), sqlc.arg('added_by'), sqlc.arg('added_at'))
ON CONFLICT (tenant_id, item_id, address) DO NOTHING;

-- name: DeleteFileLink :execrows
DELETE FROM file_link WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');
