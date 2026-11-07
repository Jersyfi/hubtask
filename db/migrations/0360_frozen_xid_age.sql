-- How many transaction IDs this database has used since vacuum last froze it (alert A-37): at
-- about two billion PostgreSQL stops accepting writes until a vacuum has run, so the age is
-- watched long before. A function because sqlc knows no catalogue relation (migration 0338's
-- reason); pg_database is readable by every role, and no row of a workspace is read. Expand only
-- (rule 12).

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION frozen_xid_age()
RETURNS bigint
LANGUAGE sql STABLE SET search_path = pg_catalog, pg_temp AS $$
  SELECT age(datfrozenxid)::bigint FROM pg_catalog.pg_database WHERE datname = current_database();
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION frozen_xid_age() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION frozen_xid_age() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
