-- A template's pages (migration 0316, configuration.md §6).

-- name: InsertTemplatePage :execrows
-- A key the hub already marks is no second page: the caller refuses it.
INSERT INTO template_page (tenant_id, page_id, hub_id, key, created_at)
VALUES (current_tenant_id(), sqlc.arg('page_id'), sqlc.arg('hub_id'), sqlc.arg('key'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, hub_id, key) DO NOTHING;

-- name: ListTemplatePages :many
-- The hub's marked pages not in the trash: a trashed one may be brought again.
SELECT t.key, t.page_id, p.title, p.icon
FROM template_page t
JOIN page p ON p.tenant_id = t.tenant_id AND p.id = t.page_id
WHERE t.tenant_id = current_tenant_id() AND t.hub_id = sqlc.arg('hub_id') AND p.deleted_at IS NULL
ORDER BY t.key
LIMIT 200;
