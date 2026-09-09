-- The system defaults' only legitimate writer is the table's owner, and FORCE was locking it out.
--
-- The reasoning is in 0002, where the same statement now stands, and in ADR-0052. This migration is
-- for the databases that applied 0002 before that line existed: without it, an installation
-- migrated last month and one created today would differ in whether their migrator can seed, which
-- is the drift forward-only migrations exist to prevent.
--
-- Idempotent, and a no-op on a database that already took it from 0002.
--
-- Expand only: one catalogue flag on one table. Nothing is rewritten, nothing is locked beyond the
-- catalogue update, and a running instance of the previous version neither notices nor cares.

-- +goose Up

ALTER TABLE item_capability_profile NO FORCE ROW LEVEL SECURITY;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
