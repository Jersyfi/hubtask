-- The compatibility floor: the oldest binary, by its newest embedded migration, that still works on
-- this schema (deployment.md §5, versioning-release.md §4). Readiness compares it with the
-- binary's own newest migration; a binary below it - a rollback across a contract step - is not
-- ready and does not start, instead of answering every write with a missing column.
--
-- **A contract migration raises it.** The step that drops what an older binary names replaces this
-- function with the number of the first migration of the release that stopped naming it. Never
-- lower: a replacement answers at least what it replaces. Expand migrations leave it alone.
--
-- A function rather than a table: the number is part of the schema the migrator owns, the
-- application role only reads it, and nothing needs a row-level-security exemption. A binary
-- from before this migration cannot read it; the floor protects the releases from this one on.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION schema_compatibility_floor()
RETURNS bigint
LANGUAGE sql IMMUTABLE SET search_path = pg_catalog, pg_temp AS $$
  SELECT 0::bigint;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION schema_compatibility_floor() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION schema_compatibility_floor() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
