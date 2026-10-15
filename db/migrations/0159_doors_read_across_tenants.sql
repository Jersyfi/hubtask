-- The doors through the boundary answer on a database the operator brings (multi-tenancy.md §2.1).
--
-- The `SECURITY DEFINER` functions that read across tenants - `resolve_tenant` (sign-in),
-- `admin_tenants`, `instance_census`, the operator register's functions, `subject_tenants`,
-- `count_provider_offers`, `move_provider_offer` - run as their owner, the role that ran the
-- migrations. `FORCE` binds that owner too, so where it is neither a superuser nor BYPASSRLS - the
-- managed database of ADR-0052 - they saw no rows: nobody could sign in.
--
-- So the owner holds standing, command-scoped policies on exactly the tables those functions read
-- across tenants, named `TO CURRENT_USER`: whoever runs this migration is whoever owns the
-- functions, on every deployment (the superuser on Compose, the chart's owner, `hubtask_migrator`
-- on a managed service). They give the owner nothing it could not give itself - it owns the tables
-- and their policies - and `hubtask_app` nothing at all: it is not named, and its policies, grants
-- and FORCE are untouched.
--
-- * `tenant`, `account`, `membership`, `legal_hold`: read (`FOR SELECT`).
-- * `identity_provider`: read, and `UPDATE` of the installation's own rows (`tenant_id IS NULL`),
--   the only rows `move_provider_offer` writes.
--
-- The period tables need nothing here: their maintenance read is the marker policy of 0068.
--
-- Forward-only and safe for a rolling update: it adds policies and removes nothing; a pod of the
-- previous release reads exactly as before. Each `CREATE POLICY` takes a short lock on its table.

-- +goose Up
-- +goose StatementBegin
DO $doors$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'tenant' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON tenant FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'account' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON account FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'membership' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON membership FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'legal_hold' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON legal_hold FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'identity_provider' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON identity_provider FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'identity_provider' AND policyname = 'doors_write') THEN
    CREATE POLICY doors_write ON identity_provider FOR UPDATE TO CURRENT_USER
      USING (tenant_id IS NULL) WITH CHECK (tenant_id IS NULL);
  END IF;
END $doors$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
