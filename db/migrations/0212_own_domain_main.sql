-- An own domain as a workspace's main address, and the way back (ADR-0095 §3, §4; UC-INS-15).
--
-- Making an own domain main moves `is_canonical` to its row; the subdomain (or assigned host) stays
-- an ACTIVE row beside it, so the API still answers there and `browser_host` sends a browser on to
-- the main address. Switching back moves it again, and the own domain then sends browsers on. A main
-- own domain that breaks hands the main address back in the check's own transaction and keeps the
-- moment in `fell_back_at`, which the workspace's administration and the installation's overview
-- warn about until the owner makes it main again or removes it.
--
-- Expand only and safe for a rolling update: the new column is nullable; `browser_host` is new;
-- `admin_own_domains` gains two columns at the end, and the previous release names the columns it
-- reads, so it reads the same. A server of the previous release makes no own domain main.

-- +goose Up
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS fell_back_at timestamptz;

-- +goose StatementBegin
-- Where a browser on this host is sent instead: the workspace's main address, for a serving row that
-- is not main beside a serving main row, where one of the two is an own domain. NULL for every other
-- host, which the request then serves as before.
CREATE OR REPLACE FUNCTION browser_host(request_host text) RETURNS text
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT m.host
  FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  JOIN tenant_host m ON m.tenant_id = h.tenant_id AND m.is_canonical AND m.state = 'ACTIVE'
  WHERE h.host = lower(rtrim(request_host, '.'))
    AND h.state = 'ACTIVE' AND NOT h.is_canonical
    AND (h.kind = 'OWN_DOMAIN' OR m.kind = 'OWN_DOMAIN')
    AND t.deleted_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

DROP FUNCTION IF EXISTS admin_own_domains();
-- +goose StatementBegin
-- Every own domain of the installation with its workspace, and every block, for the operator; with
-- when a main one fell back and the workspace's own address it fell back to.
CREATE FUNCTION admin_own_domains()
RETURNS TABLE (host text, tenant_id uuid, tenant_name text, state text, since timestamptz,
               is_main boolean, blocked_reason text, fell_back_at timestamptz, address_host text)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.host, h.tenant_id, t.display_name, h.state, coalesce(h.state_since, h.created_at),
         h.is_canonical, NULL::text, h.fell_back_at,
         (SELECT a.host FROM tenant_host a
          WHERE a.tenant_id = h.tenant_id AND a.kind = 'ADDRESS' AND a.state = 'ACTIVE'
          ORDER BY a.is_canonical DESC, a.created_at DESC LIMIT 1)
  FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.kind = 'OWN_DOMAIN' AND t.deleted_at IS NULL
  UNION ALL
  SELECT b.host, b.tenant_id, coalesce(t.display_name, ''), 'BLOCKED', b.blocked_at, b.was_main,
         b.reason, NULL::timestamptz,
         (SELECT a.host FROM tenant_host a
          WHERE a.tenant_id = b.tenant_id AND a.kind = 'ADDRESS' AND a.state = 'ACTIVE'
          ORDER BY a.is_canonical DESC, a.created_at DESC LIMIT 1)
  FROM blocked_domain b
  LEFT JOIN tenant t ON t.id = b.tenant_id AND t.deleted_at IS NULL
  ORDER BY 5 DESC, 1
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION browser_host(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION admin_own_domains() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION browser_host(text) TO hubtask_app;
GRANT EXECUTE ON FUNCTION admin_own_domains() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
