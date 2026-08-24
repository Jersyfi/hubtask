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

-- name: ListSystemCapabilityProfiles :many
-- The system defaults alone, whatever the caller's tenant has overridden.
--
-- They bound what a narrowing may do, and one question can only be answered from them: which
-- types sit directly under a collection. Read off a narrowed set, a tenant that removed a task's
-- children would promote the work package to a top level it was never allowed to sit at
-- (domain-model.md §2).
SELECT type, capabilities, allowed_child_types, max_depth
FROM item_capability_profile
WHERE tenant_id IS NULL
ORDER BY type;

-- name: ListTextLanguages :many
-- The languages this installation can index, as BCP 47 tags (C-08, ADR-0034).
--
-- Read from the database rather than listed in Go, because it is the database that decides: the
-- mapping lives in `hubtask_text_languages()`, which joins the tags this product knows against the
-- text search configurations this PostgreSQL was actually built with. A constant in the
-- application would answer for an installation that has one configuration fewer, and a client's
-- language picker would then offer a language that is silently indexed word by word.
SELECT l.tag::text FROM hubtask_text_languages() AS l(tag, configuration);
