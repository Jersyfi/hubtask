-- The profile that applies, one row per item type: a tenant's own override when it has one, the
-- system default otherwise. DISTINCT ON does the choosing in the database, so the application
-- never has to merge two lists and never has to decide which wins.
--
-- No tenant condition: row level security shows the system rows plus this tenant's, and nothing
-- else (db/schema.sql, item_capability_profile).

-- name: ListCapabilityProfiles :many
SELECT DISTINCT ON (type)
  type,
  capabilities,
  allowed_child_types,
  max_depth
FROM item_capability_profile
ORDER BY type, (tenant_id IS NOT NULL) DESC;
