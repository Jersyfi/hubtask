-- Which migrations the database has applied, for the readiness probe (deployment.md §5,
-- observability-reliability.md §5): a pod whose own migrations are not all applied is not ready.
--
-- **Numbers only, through a function.** The ledger is closed to the application role (0001_init.sql
-- revokes it: the role must not be able to rewrite the record of which migrations ran), and the
-- probe needs to read it. `SECURITY DEFINER`, owned by the migrator that owns the ledger, answering
-- the applied versions and nothing else - a caller that wanted more would have to change the
-- function, which is a migration somebody reviews. The ledger has no row level security, so this
-- reads it whether or not the owner is a superuser.
--
-- The set rather than the highest version: migrations are applied out of order (cmd/migrate allows
-- a missing one), so the highest applied number can equal a binary's while one of its migrations
-- has not run.
--
-- `plpgsql` rather than `sql`, because a `sql` body is checked when the function is created and
-- db/schema.sql is loaded without goose, where the ledger does not exist; there the function answers
-- NULL.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION applied_schema_versions()
RETURNS bigint[]
LANGUAGE plpgsql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
BEGIN
  RETURN (
    SELECT coalesce(array_agg(DISTINCT version_id ORDER BY version_id), ARRAY[]::bigint[])
    FROM public.goose_db_version
    WHERE is_applied
  );
EXCEPTION WHEN undefined_table THEN
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION applied_schema_versions() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION applied_schema_versions() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
