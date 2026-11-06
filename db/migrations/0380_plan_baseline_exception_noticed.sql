-- When a plan was first found in exception against a baseline (configuration.md §10), so that the
-- exception is recorded in the audit trail once per baseline rather than on every read of it.
--
-- Expand only: one nullable column and an UPDATE on it alone; the previous version neither writes
-- nor reads it and goes on recording every read, as before.

-- +goose Up

ALTER TABLE plan_baseline ADD COLUMN IF NOT EXISTS exception_noticed_at timestamptz;
GRANT UPDATE (exception_noticed_at) ON plan_baseline TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
