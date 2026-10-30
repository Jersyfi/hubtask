-- The code on an entry (migration 0275, UC-INT-13).

-- name: ListCodeHooks :many
SELECT id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at, last_delivery_at
FROM code_hook
WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id')
ORDER BY created_at, id;

-- name: CountCodeHooks :one
SELECT count(*)::int FROM code_hook WHERE tenant_id = current_tenant_id() AND hub_id = sqlc.arg('hub_id');

-- name: FindCodeHook :one
SELECT id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at, last_delivery_at
FROM code_hook
WHERE tenant_id = current_tenant_id() AND id = sqlc.arg('id');

-- name: PutCodeHook :exec
INSERT INTO code_hook (id, tenant_id, hub_id, kind, name, enabled, salt, run_as, created_at, updated_at, last_delivery_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('hub_id'), sqlc.arg('kind'), sqlc.arg('name'),
        sqlc.arg('enabled'), sqlc.arg('salt'), sqlc.arg('run_as'), sqlc.arg('created_at'), sqlc.arg('updated_at'),
        sqlc.narg('last_delivery_at'))
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name, enabled = EXCLUDED.enabled, salt = EXCLUDED.salt, run_as = EXCLUDED.run_as,
  updated_at = EXCLUDED.updated_at, last_delivery_at = EXCLUDED.last_delivery_at
WHERE code_hook.tenant_id = current_tenant_id();

-- name: SeeCodeDelivery :execrows
INSERT INTO code_delivery_seen (tenant_id, hook_id, digest, seen_at)
VALUES (current_tenant_id(), sqlc.arg('hook_id'), sqlc.arg('digest'), sqlc.arg('at'))
ON CONFLICT DO NOTHING;

-- name: ForgetCodeDeliveries :exec
DELETE FROM code_delivery_seen
WHERE tenant_id = current_tenant_id() AND hook_id = sqlc.arg('hook_id') AND seen_at < sqlc.arg('before');

-- name: FindCodeLink :one
SELECT id, item_id, hub_id, hook_id, kind, repository, ref, title, url, author, state, happened_at, updated_at
FROM code_link
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id') AND hook_id = sqlc.arg('hook_id')
  AND kind = sqlc.arg('kind') AND repository = sqlc.arg('repository') AND ref = sqlc.arg('ref');

-- name: PutCodeLink :exec
INSERT INTO code_link (id, tenant_id, item_id, hub_id, hook_id, kind, repository, ref, title, url, author, state,
                       happened_at, updated_at)
VALUES (sqlc.arg('id'), current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('hub_id'), sqlc.arg('hook_id'),
        sqlc.arg('kind'), sqlc.arg('repository'), sqlc.arg('ref'), sqlc.arg('title'), sqlc.arg('url'),
        sqlc.arg('author'), sqlc.arg('state'), sqlc.arg('happened_at'), sqlc.arg('updated_at'))
ON CONFLICT (id) DO UPDATE SET
  title = EXCLUDED.title, url = EXCLUDED.url, author = EXCLUDED.author, state = EXCLUDED.state,
  happened_at = EXCLUDED.happened_at, updated_at = EXCLUDED.updated_at
WHERE code_link.tenant_id = current_tenant_id();

-- name: ListCodeLinks :many
SELECT id, item_id, hub_id, hook_id, kind, repository, ref, title, url, author, state, happened_at, updated_at
FROM code_link
WHERE tenant_id = current_tenant_id() AND item_id = sqlc.arg('item_id')
ORDER BY happened_at DESC, id
LIMIT sqlc.arg('max_rows');
