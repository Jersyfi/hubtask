-- A pending deletion can be called off (UC-INS-08 check 5, multi-tenancy.md §5), and calling it off
-- gives back the workspace as it was. Three facts the row did not keep:
--
-- * `tenant.status_before_deletion` - where the workspace stood when the deletion was requested, so
--   a cancel returns a suspended workspace suspended and an active one active. NULL on a row
--   requested before this release; a cancel then returns it suspended, so nobody regains access the
--   operator did not choose.
-- * `tenant.purge_started_at` - the grace job's mark, written in the row before it removes a single
--   byte. A cancel requires it empty; the two writes meet on this one row, so a cancel can never
--   return a workspace whose media the purge already took.
-- * `automation_rule.paused_for_deletion` - which rules the deletion request switched off, so that
--   a cancel switches exactly those back on and leaves alone a rule somebody had switched off.
--
-- The listing answers the purge's mark, so the operator's screen offers *Cancel deletion* only where
-- it succeeds (P-05). Its row type changes, which `CREATE OR REPLACE` cannot do; dropped and created
-- in this one transaction, so no reader sees it missing (0118's and 0121's precedent).
--
-- Expand only and safe beside the previous release: two nullable columns and one with a constant
-- default are catalogue changes without a rewrite, and a caller of the previous release names its
-- columns of `admin_tenants()` and reads them unchanged (rule 12). An instance of the previous
-- release writes none of the three: a deletion it requests returns suspended on a cancel, the rules
-- it switches off stay off, and its grace job does not mark - bounded in SC-08's readiness record.
-- +goose Up
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS status_before_deletion tenant_status;
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS purge_started_at timestamptz;
ALTER TABLE automation_rule ADD COLUMN IF NOT EXISTS paused_for_deletion boolean NOT NULL DEFAULT false;

DROP FUNCTION IF EXISTS admin_tenants();
-- +goose StatementBegin
CREATE FUNCTION admin_tenants()
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text,
  legal_hold boolean, purge_started_at timestamptz
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT t.id, t.slug, t.display_name, t.status::text,
         t.default_locale, t.default_time_zone, t.created_at, t.purge_after,
         t.password_opened_until, t.password_opened_requester, t.password_opened_reason,
         EXISTS (SELECT 1 FROM legal_hold h WHERE h.tenant_id = t.id AND h.released_at IS NULL),
         t.purge_started_at
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
