-- Plans: a level between the installation and its workspaces (multi-tenancy.md §4.2, ADR-0086).
--
-- `plan` is one row per plan; `plan_setting` holds what a plan decides in the installation level's
-- own key shape (`sign_in.min_length`, `quota.items`, `ai.sources`) plus `entitlement.<feature>`
-- for a feature the plan leaves out, so one mapping reads both levels. A plan holds only what it
-- decides; nothing is copied into a workspace, which references its plan through
-- `tenant.plan_id` (0099 left the column for this).
--
-- **No row level security, like `instance_setting`.** Every workspace has to read its plan once a
-- workspace's values resolve through it, so a policy comparing current_tenant_id() would make every plan
-- invisible to everybody; the tables hold configuration, never a person's data, and what bounds
-- writing them is the control plane's use cases behind `admin:tenants` and the register. Entered in
-- the three boundary lists in this commit.
--
-- **A short name is never reused.** A removed plan keeps its row with `removed_at`, and the
-- application role holds no DELETE on `plan`: the unique name stays taken, which is what a
-- platform that mapped its products to it relies on (UC-INS-13 check 9).
--
-- **Removing a plan in use is refused in one door.** `tenant` is FORCE-d behind `tenant_self`, so
-- the application role cannot see which workspaces are on a plan, and a soft removal fires no
-- `RESTRICT`. `remove_plan` locks the plan row, then counts in a statement of its own - a fresh
-- snapshot under READ COMMITTED - so a move that committed first is counted, and a move that comes
-- later waits on the lock and then finds the plan gone. It answers one word and no row; its owner
-- reads `tenant` through the standing `doors_read` policy (0159).
--
-- `admin_tenants()` gains the plan's short name and the workspace's version: its row type
-- changes, which `CREATE OR REPLACE` cannot do, so it is dropped and created in this one
-- transaction and no reader sees it missing (0118, 0121). A caller of the previous release names
-- its columns and reads the old ones unchanged.
--
-- Expand only and safe for a rolling update (rule 12): two new tables, a reference on a column
-- every row holds NULL in, added NOT VALID and validated in the same step (it reads no row that
-- could fail), a new function, and a listing with two more columns.

-- +goose Up
CREATE TABLE IF NOT EXISTS plan (
  id           uuid PRIMARY KEY,
  name         text NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9-]{1,39}$'),
  display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 200),
  version      integer NOT NULL DEFAULT 1,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  -- Who changed it last, recorded rather than enforced, as `instance_setting` does: an account is
  -- a tenant's row and this table is nobody's.
  updated_by   uuid,
  removed_at   timestamptz,
  CONSTRAINT plan_name_key UNIQUE (name)
);

GRANT SELECT, INSERT, UPDATE ON plan TO hubtask_app;

CREATE TABLE IF NOT EXISTS plan_setting (
  plan_id uuid NOT NULL REFERENCES plan (id) ON DELETE RESTRICT,
  key     text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
  value   jsonb NOT NULL,
  locked  boolean NOT NULL DEFAULT false,
  PRIMARY KEY (plan_id, key)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON plan_setting TO hubtask_app;

-- +goose StatementBegin
DO $reference$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'tenant_plan_fk') THEN
    ALTER TABLE tenant ADD CONSTRAINT tenant_plan_fk
      FOREIGN KEY (plan_id) REFERENCES plan (id) ON DELETE RESTRICT NOT VALID;
  END IF;
END $reference$;
-- +goose StatementEnd
ALTER TABLE tenant VALIDATE CONSTRAINT tenant_plan_fk;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION remove_plan(removed_plan uuid, at timestamptz, actor uuid)
RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM 1 FROM plan WHERE id = removed_plan AND removed_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN
    RETURN 'NOT_FOUND';
  END IF;
  IF EXISTS (SELECT 1 FROM tenant WHERE plan_id = removed_plan AND deleted_at IS NULL) THEN
    RETURN 'IN_USE';
  END IF;
  UPDATE plan
     SET removed_at = at, updated_at = at, updated_by = actor, version = version + 1
   WHERE id = removed_plan;
  RETURN 'REMOVED';
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION remove_plan(uuid, timestamptz, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION remove_plan(uuid, timestamptz, uuid) TO hubtask_app;

DROP FUNCTION IF EXISTS admin_tenants();
-- +goose StatementBegin
CREATE FUNCTION admin_tenants()
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text,
  legal_hold boolean, purge_started_at timestamptz,
  plan_name text, version integer
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT t.id, t.slug, t.display_name, t.status::text,
         t.default_locale, t.default_time_zone, t.created_at, t.purge_after,
         t.password_opened_until, t.password_opened_requester, t.password_opened_reason,
         EXISTS (SELECT 1 FROM legal_hold h WHERE h.tenant_id = t.id AND h.released_at IS NULL),
         t.purge_started_at,
         p.name, t.version
  FROM tenant t
  LEFT JOIN plan p ON p.id = t.plan_id
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
