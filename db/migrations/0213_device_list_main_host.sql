-- The device's list of workspaces (ADR-0095 §7; UC-ID-25): whether a host is a workspace's main
-- address, asked before the installation's own address keeps an entry and whenever it shows one.
--
-- Expand only and safe for a rolling update: one new function, which the previous release does not
-- call.

-- +goose Up
-- +goose StatementBegin
-- Whether this host is the main address of a live workspace. A yes or a no - never the workspace -
-- so the question names nothing to somebody who did not already hold a signed entry for the host.
CREATE OR REPLACE FUNCTION is_main_host(request_host text) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT EXISTS (
    SELECT 1
    FROM tenant_host h
    JOIN tenant t ON t.id = h.tenant_id
    WHERE h.host = lower(rtrim(request_host, '.'))
      AND h.is_canonical AND h.state = 'ACTIVE'
      AND t.deleted_at IS NULL
  )
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION is_main_host(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION is_main_host(text) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
