-- A purchase platform's own reference on a workspace, and the query it finds workspaces with
-- (multi-tenancy.md §4.3, UC-INS-13 check 10).
--
-- `external_reference` is the platform's customer or subscription id, opaque to Hubtask: it is what
-- a platform finds a workspace by once a provisioning answer is lost and its key has expired. Its
-- uniqueness is the next migration's index, built CONCURRENTLY outside a transaction.
--
-- `admin_tenants_query` is the control plane's filtered, paged read: the filters and the keyset on
-- (created_at, id) inside the function, so that a page reads its own rows rather than every
-- workspace on the installation - a SECURITY DEFINER function is not inlined, and filtering
-- `admin_tenants()` from outside would materialise them all per page. A new function rather than a
-- recreated one: `admin_tenants()` keeps its row type, and nothing is dropped. Like it, it answers
-- what the control plane may already list, behind `admin:tenants` and the register; it reads
-- `tenant`, `plan` and `legal_hold` through the owner's standing policies (0159).
--
-- Expand only and safe for a rolling update (rule 12): a nullable column, added under a short lock
-- timeout - every request reads `tenant`, so the statement fails rather than queueing them - and a
-- new function. A pod of the previous release neither reads nor writes the column.

-- +goose Up
SET LOCAL lock_timeout = '5s';
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS external_reference text;
-- The bound NOT VALID and validated in its own statement (versioning-release.md §4): the validation
-- scans without the exclusive lock, and every row is NULL.
-- +goose StatementBegin
DO $bound$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenant_external_reference_length') THEN
    ALTER TABLE tenant ADD CONSTRAINT tenant_external_reference_length
      CHECK (external_reference IS NULL OR char_length(external_reference) BETWEEN 1 AND 200) NOT VALID;
  END IF;
END $bound$;
-- +goose StatementEnd
ALTER TABLE tenant VALIDATE CONSTRAINT tenant_external_reference_length;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION admin_tenants_query(
  want_id uuid, want_slug text, want_reference text, want_plan text, want_without_plan boolean,
  want_status text, after_at timestamptz, after_id uuid, page_size integer
)
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text,
  legal_hold boolean, purge_started_at timestamptz,
  plan_name text, version integer, external_reference text
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT t.id, t.slug, t.display_name, t.status::text,
         t.default_locale, t.default_time_zone, t.created_at, t.purge_after,
         t.password_opened_until, t.password_opened_requester, t.password_opened_reason,
         EXISTS (SELECT 1 FROM legal_hold h WHERE h.tenant_id = t.id AND h.released_at IS NULL),
         t.purge_started_at,
         p.name, t.version, t.external_reference
  FROM tenant t
  LEFT JOIN plan p ON p.id = t.plan_id
  WHERE t.deleted_at IS NULL
    AND (want_id IS NULL OR t.id = want_id)
    AND (want_slug IS NULL OR t.slug = want_slug)
    AND (want_reference IS NULL OR t.external_reference = want_reference)
    AND (want_plan IS NULL OR p.name = want_plan)
    AND (NOT coalesce(want_without_plan, false) OR t.plan_id IS NULL)
    AND (want_status IS NULL OR t.status::text = want_status)
    AND (after_at IS NULL OR (t.created_at, t.id) > (after_at, after_id))
  ORDER BY t.created_at, t.id
  LIMIT page_size
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION admin_tenants_query(uuid, text, text, text, boolean, text, timestamptz, uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants_query(uuid, text, text, text, boolean, text, timestamptz, uuid, integer) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
