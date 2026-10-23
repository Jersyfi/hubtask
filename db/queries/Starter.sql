-- The first start's question (UC-WRK-30 check 5): asked once, while nobody has made anything in the
-- workspace yet. The tenant is the transaction's own, as everywhere: current_tenant_id().

-- name: StarterPending :one
-- The hub and collection a workspace is created with (multi-tenancy.md §5) are written in the
-- tenant row's own moment, so a container from any other moment, or any entry, is something
-- somebody made. Trashed and archived ones count: a workspace that ever held one is not new.
SELECT (t.starter_answered_at IS NULL
        AND NOT EXISTS (SELECT 1 FROM container c WHERE c.created_at <> t.created_at)
        AND NOT EXISTS (SELECT 1 FROM work_item))::boolean AS pending
FROM tenant t
WHERE t.id = current_tenant_id();

-- name: CloseStarter :exec
UPDATE tenant SET starter_answered_at = sqlc.arg('at')
WHERE id = current_tenant_id() AND starter_answered_at IS NULL;
