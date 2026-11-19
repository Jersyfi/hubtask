-- A dashboard's filter (work-surface concept §6.5): a Hubtask query every QUERY widget inherits,
-- names already identifiers, bounded by the domain as a query line is.
--
-- Expand only (rule 12): a text with a constant default writes no row. A pod of the previous
-- release never reads the column, and a dashboard it saves keeps the filter it had.
-- +goose Up
ALTER TABLE dashboard ADD COLUMN IF NOT EXISTS filter text NOT NULL DEFAULT '';

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
