-- The lifecycle of a tenant (H-06, multi-tenancy.md §5): the grace deadline on the row, the
-- installation's own evidence journal, the one legitimate enumerator, and the narrow purge of a
-- trail that dies with its tenant.

-- Forward-only and safe for a rolling update: one nullable column, one new table nothing old
-- touches, and two SECURITY DEFINER functions.

-- +goose Up

-- When the 30-day grace runs out. Set by the deletion request's own write; the hard-delete job
-- it seeds waits for this moment.
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS purge_after timestamptz;

-- The instance's own journal (H-06, the angle audit.md never had to answer before): evidence of
-- acts whose per-tenant trail cannot hold them - above all, a hard delete, after which the
-- tenant's own audit chain is gone by design. Identifiers, a slug, counts and moments; never
-- content.
--
-- Deliberately without a row-level-security policy, the job table's precedent: the rows belong
-- to the installation, not to any tenant, and a policy comparing against current_tenant_id()
-- would make them unreachable under every honest scope (the privacy_incident lesson). What
-- bounds it instead: the application writes it in exactly one place, and reading it is the
-- operator's, through the database - no API serves it in this milestone.
CREATE TABLE IF NOT EXISTS instance_event (
  id          uuid PRIMARY KEY,
  occurred_at timestamptz NOT NULL,
  action      text NOT NULL,
  -- The tenant the act was about. A bare identifier, no foreign key: the row it named is
  -- usually gone, which is the reason this table exists.
  tenant_id   uuid,
  tenant_slug text,
  actor_label text,
  details     jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS instance_event_occurred_idx ON instance_event (occurred_at);

-- Append-only for the application, the audit trail's discipline: evidence that could be edited
-- or removed afterwards would not be evidence. The default privileges would hand the role more.
REVOKE UPDATE, DELETE, TRUNCATE ON instance_event FROM hubtask_app;
GRANT SELECT, INSERT ON instance_event TO hubtask_app;

-- The one legitimate tenant enumerator (0.6.0 decision 6): provisioning and lifecycle are the
-- control plane's job, and the control plane must see its rows. SECURITY DEFINER for
-- resolve_tenant's reason; what bounds it is the application - the use case behind it demands
-- the admin:tenants scope, which no session carries.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION admin_tenants()
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT id, slug, display_name, status::text,
         default_locale, default_time_zone, created_at, purge_after
  FROM tenant
  WHERE deleted_at IS NULL
  ORDER BY created_at, id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION admin_tenants() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants() TO hubtask_app;

-- The trail that dies with its tenant. audit_log rows carry no foreign key to tenant (the
-- partitions predate it) and the application role deliberately holds no DELETE on them (T-15) -
-- both correct for a living tenant, and both in the way of a hard delete whose promise is that
-- the tenant's data is gone. This is the one narrow act that squares them: it removes exactly
-- one tenant's rows, it exists for the hard delete alone, and every use is preceded by the
-- instance_event entry that outlives it.
-- The trail's immutability trigger learns the one exception it now has (level 2 of audit.md
-- §3): a row may fall only while the transaction-scoped purge marker names exactly its tenant -
-- and the only writer of that marker is purge_tenant_trail below, which closes the window
-- before it returns. UPDATE stays impossible unconditionally.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE'
     AND current_setting('hubtask.trail_purge', true) = OLD.tenant_id::text THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'audit_log is append-only (attempted %)', TG_OP
    USING ERRCODE = 'insufficient_privilege';
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION purge_tenant_trail(purged_tenant uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  removed bigint;
BEGIN
  -- Only the partitioned trail itself: audit_anchor and audit_pseudonym carry foreign keys and
  -- die with the tenant row. The marker opens the immutability trigger for exactly this tenant,
  -- and the window closes before the function returns.
  PERFORM set_config('hubtask.trail_purge', purged_tenant::text, true);
  DELETE FROM audit_log WHERE tenant_id = purged_tenant;
  GET DIAGNOSTICS removed = ROW_COUNT;
  PERFORM set_config('hubtask.trail_purge', '', true);
  RETURN removed;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION purge_tenant_trail(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION purge_tenant_trail(uuid) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
