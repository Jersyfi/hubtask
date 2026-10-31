-- A template's sample entries (migration 0307, configuration.md §6).

-- name: InsertSampleEntry :execrows
-- A key the hub already marks is no second sample: the caller refuses it.
INSERT INTO sample_entry (tenant_id, item_id, hub_id, key, created_at)
VALUES (current_tenant_id(), sqlc.arg('item_id'), sqlc.arg('hub_id'), sqlc.arg('key'), sqlc.arg('created_at'))
ON CONFLICT (tenant_id, hub_id, key) DO NOTHING;

-- name: ListSampleEntries :many
-- The hub's marked entries still live: one in the trash is no longer a sample anybody tries.
SELECT s.key, s.item_id, w.title, w.entry_type, w.collection_id
FROM sample_entry s
JOIN work_item w ON w.tenant_id = s.tenant_id AND w.id = s.item_id
WHERE s.tenant_id = current_tenant_id() AND s.hub_id = sqlc.arg('hub_id') AND w.deleted_at IS NULL
ORDER BY s.key
LIMIT 200;
