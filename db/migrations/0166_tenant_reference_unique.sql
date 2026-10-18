-- A platform's reference names one workspace (multi-tenancy.md §4.3): unique among the workspaces
-- that have one.
--
-- An index rather than a check before the write: two provisionings with one reference at the same
-- moment would both find it free. A unique index is checked whatever row level security hides, so
-- it holds across tenants although `tenant_self` lets a transaction see only its own row - and the
-- refusal names no other workspace (P-01).
--
-- **No transaction**, because the index is built CONCURRENTLY (versioning-release.md §4) on a table
-- every request reads. A failed build leaves an invalid index under the name, which IF NOT EXISTS
-- would take as done, so a run drops whatever stands first: the migration heals on retry (0149).
-- Expand only (rule 12): nothing written by the previous release carries a reference.

-- +goose NO TRANSACTION

-- +goose Up
DROP INDEX CONCURRENTLY IF EXISTS tenant_external_reference_key;

CREATE UNIQUE INDEX CONCURRENTLY tenant_external_reference_key
  ON tenant (external_reference) WHERE external_reference IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
