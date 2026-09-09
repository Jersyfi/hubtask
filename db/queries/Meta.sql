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

-- name: SemanticSearchAvailable :one
-- Whether this installation can search by meaning (J-09, ADR-0050).
--
-- The *table's* existence rather than the extension's, and the difference matters: an installation
-- where somebody installed pgvector after the migrations ran has the extension and no store, which
-- is not a working semantic search. One question, asked of the object the search actually reads.
SELECT (to_regclass('public.item_embedding') IS NOT NULL)::boolean AS available;

-- name: FindWorkspace :one
-- The tenant's own row, read from inside the tenant (F4-01). No tenant parameter: row level
-- security has already bound the transaction to exactly one, which is what makes another
-- workspace invisible rather than forbidden (ADR-0010).
SELECT id, slug, display_name, status, default_locale, default_time_zone,
       settings, created_at, updated_at, version
FROM tenant
WHERE id = current_tenant_id() AND deleted_at IS NULL;

-- name: UpdateWorkspace :execrows
-- The three columns and the settings keys this build models, guarded on the row version so that
-- two administrators changing one workspace see each other. The settings document is merged
-- rather than replaced: `||` keeps every key this version does not know, which is what stops an
-- older binary from discarding what a newer one wrote.
UPDATE tenant
SET display_name = sqlc.arg('display_name'),
    default_locale = sqlc.arg('default_locale'),
    default_time_zone = sqlc.arg('default_time_zone'),
    settings = settings || sqlc.arg('settings')::jsonb,
    updated_at = sqlc.arg('now'), version = version + 1
WHERE id = current_tenant_id() AND deleted_at IS NULL
  AND version = sqlc.arg('expected_version');
