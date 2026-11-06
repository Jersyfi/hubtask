-- A workspace's own domain, proven by DNS (ADR-0095 §4, §5; UC-INS-15).
--
-- An own domain is a `tenant_host` row of its own kind: PENDING until a CNAME points it at the
-- installation's target and a TXT record at `_hubtask.<domain>` carries the row's mark, VERIFIED
-- once both are found, ACTIVE once the ingress asked for its certificate and a request reached the
-- workspace through it, BROKEN once its proof is gone. A VERIFIED row answers as well as an ACTIVE
-- one: the first request over HTTPS is what makes it ACTIVE. One per workspace (Decision 9).
--
-- A block is the installation's and outlives the row: `blocked_domain` keeps the host, so no
-- workspace sets it up again, and the workspace it was taken from keeps seeing why until its owner
-- removes it from view. The reason is the operator's text, shown to that owner word for word.
--
-- The doors run as their owner, whom FORCE binds (0159, 0207): the owner reads every row, and now
-- updates one through `doors_update`. `hubtask_app` gains the functions, the table under its
-- workspace's policy, and nothing across workspaces.
--
-- Expand only and safe for a rolling update: every new column has a default or is nullable, a row
-- of the previous release is an `ADDRESS`, and `resolve_tenant_host` answers what it answered
-- before for every such row. A server of the previous release writes no own domain.

-- +goose Up
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'ADDRESS';
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS state_since timestamptz;
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS checked_at timestamptz;
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS target_found boolean NOT NULL DEFAULT false;
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS proof_found boolean NOT NULL DEFAULT false;
ALTER TABLE tenant_host ADD COLUMN IF NOT EXISTS certificate_asked_at timestamptz;

-- +goose StatementBegin
DO $kind$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenant_host_kind_check') THEN
    ALTER TABLE tenant_host ADD CONSTRAINT tenant_host_kind_check
      CHECK (kind IN ('ADDRESS', 'OWN_DOMAIN'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'tenant_host' AND policyname = 'doors_update') THEN
    CREATE POLICY doors_update ON tenant_host FOR UPDATE TO CURRENT_USER USING (true) WITH CHECK (true);
  END IF;
END $kind$;
-- +goose StatementEnd

CREATE UNIQUE INDEX IF NOT EXISTS tenant_host_own_domain_uq
  ON tenant_host (tenant_id) WHERE kind = 'OWN_DOMAIN';

CREATE TABLE IF NOT EXISTS blocked_domain (
  host         text PRIMARY KEY CHECK (host = lower(host) AND length(host) BETWEEN 4 AND 253),
  -- The workspace it was taken from; NULL once that workspace is gone.
  tenant_id    uuid REFERENCES tenant(id) ON DELETE SET NULL,
  reason       text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
  blocked_at   timestamptz NOT NULL,
  -- Whether the domain was the workspace's main address when it was blocked.
  was_main     boolean NOT NULL DEFAULT false,
  -- When the workspace's owner removed it from view; the block stays.
  dismissed_at timestamptz
);
ALTER TABLE blocked_domain ENABLE ROW LEVEL SECURITY;
ALTER TABLE blocked_domain FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'blocked_domain' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON blocked_domain
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'blocked_domain' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON blocked_domain FOR SELECT TO CURRENT_USER USING (true);
  END IF;
END $policy$;
-- +goose StatementEnd
GRANT SELECT, INSERT, UPDATE ON blocked_domain TO hubtask_app;

-- +goose StatementBegin
-- Only a serving row of a live workspace answers: an ACTIVE one, or a VERIFIED own domain waiting
-- for its first request. The signature is 0207's, so a server of the previous release asks the same.
CREATE OR REPLACE FUNCTION resolve_tenant_host(request_host text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.tenant_id FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.host = lower(rtrim(request_host, '.'))
    AND (h.state = 'ACTIVE' OR (h.kind = 'OWN_DOMAIN' AND h.state = 'VERIFIED'))
    AND t.deleted_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- The ingress's question on the operations port: may this host have a certificate? Yes for a
-- serving row of a live workspace, never for a blocked, pending or broken one. An own domain is
-- stamped as asked, which is half of what makes it ACTIVE.
CREATE OR REPLACE FUNCTION own_domain_certificate(request_host text, asked_at timestamptz) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE
  wanted text := lower(rtrim(coalesce(request_host, ''), '.'));
  found  boolean;
BEGIN
  SELECT true INTO found FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.host = wanted
    AND (h.state = 'ACTIVE' OR (h.kind = 'OWN_DOMAIN' AND h.state = 'VERIFIED'))
    AND t.deleted_at IS NULL
    AND NOT EXISTS (SELECT 1 FROM blocked_domain b WHERE b.host = wanted);
  IF NOT coalesce(found, false) THEN
    RETURN false;
  END IF;
  UPDATE tenant_host SET certificate_asked_at = asked_at
  WHERE host = wanted AND kind = 'OWN_DOMAIN' AND certificate_asked_at IS NULL;
  RETURN true;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- A request reached a workspace through this host: a VERIFIED own domain the ingress asked about
-- becomes ACTIVE. Whether it awaits that is answered beside the workspace, so the request path
-- writes only when there is something to write.
CREATE OR REPLACE FUNCTION resolve_serving_host(request_host text)
RETURNS TABLE (tenant_id uuid, awaits_request boolean)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.tenant_id,
         (h.kind = 'OWN_DOMAIN' AND h.state = 'VERIFIED' AND h.certificate_asked_at IS NOT NULL)
  FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.host = lower(rtrim(request_host, '.'))
    AND (h.state = 'ACTIVE' OR (h.kind = 'OWN_DOMAIN' AND h.state = 'VERIFIED'))
    AND t.deleted_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION own_domain_served(request_host text, served_at timestamptz) RETURNS boolean
LANGUAGE sql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
  WITH moved AS (
    UPDATE tenant_host SET state = 'ACTIVE', state_since = served_at
    WHERE host = lower(rtrim(request_host, '.'))
      AND kind = 'OWN_DOMAIN' AND state = 'VERIFIED' AND certificate_asked_at IS NOT NULL
    RETURNING 1
  )
  SELECT EXISTS (SELECT 1 FROM moved)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Whether the operator blocked a host, asked from the workspace that wants to set it up.
CREATE OR REPLACE FUNCTION own_domain_blocked(request_host text) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT EXISTS (SELECT 1 FROM blocked_domain WHERE host = lower(rtrim(request_host, '.')))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- The own domains the check owes a look: pending and broken ones not looked at since
-- `pending_before`, serving ones not since `serving_before`; the longest unchecked first.
CREATE OR REPLACE FUNCTION own_domains_due(pending_before timestamptz, serving_before timestamptz, most integer)
RETURNS TABLE (tenant_id uuid, host text, verification text, state text)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.tenant_id, h.host, h.verification, h.state
  FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.kind = 'OWN_DOMAIN'
    AND t.deleted_at IS NULL
    AND (h.checked_at IS NULL
         OR (h.state IN ('PENDING', 'BROKEN') AND h.checked_at < pending_before)
         OR (h.state IN ('VERIFIED', 'ACTIVE') AND h.checked_at < serving_before))
  ORDER BY h.checked_at NULLS FIRST, h.host
  LIMIT most
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Every own domain of the installation with its workspace, and every block, for the operator.
CREATE OR REPLACE FUNCTION admin_own_domains()
RETURNS TABLE (host text, tenant_id uuid, tenant_name text, state text, since timestamptz,
               is_main boolean, blocked_reason text)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.host, h.tenant_id, t.display_name, h.state, coalesce(h.state_since, h.created_at),
         h.is_canonical, NULL::text
  FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.kind = 'OWN_DOMAIN' AND t.deleted_at IS NULL
  UNION ALL
  SELECT b.host, b.tenant_id, coalesce(t.display_name, ''), 'BLOCKED', b.blocked_at, b.was_main,
         b.reason
  FROM blocked_domain b
  LEFT JOIN tenant t ON t.id = b.tenant_id AND t.deleted_at IS NULL
  ORDER BY 5 DESC, 1
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION own_domain_certificate(text, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION resolve_serving_host(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION own_domain_served(text, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION own_domain_blocked(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION own_domains_due(timestamptz, timestamptz, integer) FROM PUBLIC;
REVOKE ALL ON FUNCTION admin_own_domains() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION own_domain_certificate(text, timestamptz) TO hubtask_app;
GRANT EXECUTE ON FUNCTION resolve_serving_host(text) TO hubtask_app;
GRANT EXECUTE ON FUNCTION own_domain_served(text, timestamptz) TO hubtask_app;
GRANT EXECUTE ON FUNCTION own_domain_blocked(text) TO hubtask_app;
GRANT EXECUTE ON FUNCTION own_domains_due(timestamptz, timestamptz, integer) TO hubtask_app;
GRANT EXECUTE ON FUNCTION admin_own_domains() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
