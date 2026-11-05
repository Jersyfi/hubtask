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

-- name: CloseStarter :execrows
-- One row where this transaction closed it, none where it was closed already: the row lock makes a
-- second answer arriving at once wait, and then find it closed.
UPDATE tenant SET starter_answered_at = sqlc.arg('at')
WHERE id = current_tenant_id() AND starter_answered_at IS NULL;

-- name: StarterSkipped :one
-- A skipped question may still be answered while the workspace holds nothing somebody made, by the
-- same test as StarterPending: a skip made nothing, an answer made a hub.
SELECT (t.starter_answered_at IS NOT NULL
        AND NOT EXISTS (SELECT 1 FROM container c WHERE c.created_at <> t.created_at)
        AND NOT EXISTS (SELECT 1 FROM work_item))::boolean AS skipped
FROM tenant t
WHERE t.id = current_tenant_id();

-- name: HoldStarter :exec
-- The tenant row's lock, taken before the question is read: a second set arriving at once waits for
-- the first and then finds the hub it made, whether or not the question was skipped before.
SELECT 1 FROM tenant WHERE id = current_tenant_id() FOR UPDATE;
