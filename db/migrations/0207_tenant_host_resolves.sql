-- A request's host names its workspace (ADR-0095 §1, multi-tenancy.md §3).
--
-- `tenant_host` becomes the resolver in multi mode: a host names a workspace through one narrow
-- door that answers one identifier or none, `resolve_tenant`'s shape. Only an ACTIVE row of a
-- workspace that is not deleted answers; a PENDING, VERIFIED or BROKEN row is a host nobody serves.
--
-- Every workspace needs its main row before anything resolves through the table, and a workspace
-- provisioned before migration 0104 has none. This migration cannot write it: the row is the short
-- name under the installation's own host, which is configuration the server reads at start and the
-- migrator never sees (0104's reason). So it adds `ensure_main_hosts(base)`, which the server calls
-- at every start in multi mode: one main row, ACTIVE and canonical, for each live workspace that
-- has none, and a count back - never a row, never a name. Idempotent, and two servers starting at
-- once write each row once (the unique indexes, `ON CONFLICT DO NOTHING`).
--
-- The doors run as their owner, whom FORCE binds (0159): the owner reads `tenant_host` across
-- workspaces and inserts into it, by standing policies named `TO CURRENT_USER`. `hubtask_app`
-- gains nothing but the two functions.
--
-- Expand only and safe for a rolling update: a server of the previous release resolves by short
-- name exactly as before, and a workspace it provisions gets its row as it always has.

-- +goose Up
-- +goose StatementBegin
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'tenant_host' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON tenant_host FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'tenant_host' AND policyname = 'doors_write') THEN
    CREATE POLICY doors_write ON tenant_host FOR INSERT TO CURRENT_USER WITH CHECK (true);
  END IF;
END $owner$;
-- +goose StatementEnd

-- +goose StatementBegin
-- The one question a request asks before it has a credential: which workspace does this host name?
CREATE OR REPLACE FUNCTION resolve_tenant_host(request_host text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.tenant_id FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.host = lower(rtrim(request_host, '.'))
    AND h.state = 'ACTIVE'
    AND t.deleted_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION resolve_tenant_host(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_tenant_host(text) TO hubtask_app;

-- +goose StatementBegin
-- The main row of every live workspace that has none, under the base host the server names. The
-- mark is drawn here because the row never presents it (0104); `gen_random_uuid` is PostgreSQL's
-- own. A base that makes no host - empty, or too long for one - writes nothing.
CREATE OR REPLACE FUNCTION ensure_main_hosts(base_host text) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE
  base    text := lower(rtrim(coalesce(base_host, ''), '.'));
  written integer;
BEGIN
  IF base = '' OR position('.' in base) = 1 THEN
    RETURN 0;
  END IF;
  INSERT INTO tenant_host (tenant_id, host, state, verification, verified_at, is_canonical, created_at)
  SELECT t.id, lower(t.slug) || '.' || base, 'ACTIVE',
         'hbt-dv-' || replace(gen_random_uuid()::text, '-', ''), now(), true, now()
  FROM tenant t
  WHERE t.deleted_at IS NULL
    AND length(lower(t.slug) || '.' || base) BETWEEN 4 AND 253
    AND NOT EXISTS (SELECT 1 FROM tenant_host h WHERE h.tenant_id = t.id AND h.is_canonical)
  ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS written = ROW_COUNT;
  RETURN written;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION ensure_main_hosts(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_main_hosts(text) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
