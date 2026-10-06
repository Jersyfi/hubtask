-- An operator opens the password for one workspace (ADR-0078 §3, SC-34).
--
-- For a provider that is switched on but broken - unreachable, or admitting nobody - the operator
-- opens the password for one named workspace for a limited time. It is ADR-0076 §4's fallback with a
-- person's decision as its cause: the password opens for every account that holds one, under the
-- workspace's own rules and second factor, whatever the workspace's own switch and any installation
-- lock say.
--
-- `password_opened_until` is when the opening ends. It is honoured where it is read: past it, the
-- opening is over, whether or not anything has cleared the row yet - no job is needed for the
-- password to close. The job the opening seeds clears the three columns afterwards and tells the
-- workspace's administrators, which is bookkeeping rather than the end itself.
--
-- `password_opened_requester` and `password_opened_reason` are what the operator was told: who asked
-- (free text - a ticket reference rather than a name, where the operator can) and why. They are on the
-- row only while the opening stands and are cleared with it; the trail and the journal keep their own
-- copies under their own retention (docs/privacy/data-catalog.md).
--
-- None of the three is in the settings document, so the workspace's own `PATCH /tenant` - which
-- writes three columns and merges `settings` - cannot reach them. The opening is the control plane's.
-- +goose Up
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS password_opened_until timestamptz;
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS password_opened_requester text
  CHECK (length(password_opened_requester) BETWEEN 1 AND 200);
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS password_opened_reason text
  CHECK (length(password_opened_reason) BETWEEN 1 AND 500);
-- An opening is whole or absent: an end without a reason, or a reason without an end, is a row
-- nobody can read back as either.
ALTER TABLE tenant ADD CONSTRAINT tenant_password_opening_whole_check
  CHECK (num_nulls(password_opened_until, password_opened_requester, password_opened_reason) IN (0, 3));

-- The enumerator answers the opening beside the lifecycle, so the installation's screen can show
-- which workspace has one in force (SC-34). Its row type changes, which `CREATE OR REPLACE` cannot
-- do; dropped and created in this one transaction, so no reader sees it missing. A caller that
-- names its columns - every one there is - reads the old ones unchanged.
DROP FUNCTION IF EXISTS admin_tenants();
-- +goose StatementBegin
CREATE FUNCTION admin_tenants()
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT id, slug, display_name, status::text,
         default_locale, default_time_zone, created_at, purge_after,
         password_opened_until, password_opened_requester, password_opened_reason
  FROM tenant
  WHERE deleted_at IS NULL
  ORDER BY created_at, id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION admin_tenants() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
