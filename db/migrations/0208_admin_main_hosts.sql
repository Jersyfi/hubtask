-- The control plane reads each workspace's main address (ADR-0095 §6, UC-INS-20).
--
-- An installation that assigns hosts names a workspace by its host as much as by its short name:
-- the operator's list shows it, and *New workspace* says a host already in that list while it is
-- typed. `admin_main_hosts(want)` answers the main row - canonical and ACTIVE - of each live
-- workspace among `want`, or of every live workspace where `want` is NULL: an identifier and a host,
-- nothing else. It answers what the control plane may already list, behind `admin:tenants` and the
-- register, and reads `tenant_host` through the owner's standing policy (0207).
--
-- A new function rather than a recreated `admin_tenants()`: nothing is dropped and no row type
-- changes, so the migration is expand only and safe for a rolling update (rule 12) - a server of the
-- previous release never calls it.

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION admin_main_hosts(want uuid[]) RETURNS TABLE (tenant_id uuid, host text)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.tenant_id, h.host
  FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.is_canonical
    AND h.state = 'ACTIVE'
    AND t.deleted_at IS NULL
    AND (want IS NULL OR h.tenant_id = ANY (want))
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION admin_main_hosts(uuid[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_main_hosts(uuid[]) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
