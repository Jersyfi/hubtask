-- The installation's listing says whether a legal hold is in force in a workspace (data-protection.md
-- §5): a workspace under a hold is not deleted, and the operator's screen leaves *Delete* out for it
-- and says why, rather than offering a button the server refuses (P-05).
--
-- A boolean and nothing else - no scope, no count, no reason: the operator needs to know that the
-- deletion cannot proceed, not what the workspace is preserving or about whom (P-01). The function is
-- SECURITY DEFINER, so the subquery reads `legal_hold` past row level security; it answers one state
-- per workspace the enumerator already lists.
--
-- Its row type changes, which `CREATE OR REPLACE` cannot do; dropped and created in this one
-- transaction, so no reader sees it missing (0118's precedent). A caller of the previous release
-- names its columns and reads the old ones unchanged (rule 12).
-- +goose Up
DROP FUNCTION IF EXISTS admin_tenants();
-- +goose StatementBegin
CREATE FUNCTION admin_tenants()
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text,
  legal_hold boolean
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT t.id, t.slug, t.display_name, t.status::text,
         t.default_locale, t.default_time_zone, t.created_at, t.purge_after,
         t.password_opened_until, t.password_opened_requester, t.password_opened_reason,
         EXISTS (SELECT 1 FROM legal_hold h WHERE h.tenant_id = t.id AND h.released_at IS NULL)
  FROM tenant t
  WHERE t.deleted_at IS NULL
  ORDER BY t.created_at, t.id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION admin_tenants() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
