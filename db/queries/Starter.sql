-- The first start's question (UC-WRK-30 check 5): asked once, while the workspace holds no hub.
-- The tenant is the transaction's own, as everywhere: current_tenant_id().

-- name: StarterPending :one
-- A trashed or archived hub counts: a workspace that ever held one is not new.
SELECT (t.starter_answered_at IS NULL
        AND NOT EXISTS (SELECT 1 FROM container c WHERE c.type = 'HUB'))::boolean AS pending
FROM tenant t
WHERE t.id = current_tenant_id();

-- name: CloseStarter :exec
UPDATE tenant SET starter_answered_at = sqlc.arg('at')
WHERE id = current_tenant_id() AND starter_answered_at IS NULL;
