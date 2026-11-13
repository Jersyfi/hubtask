-- An automation's *for which entries* (work-surface concept E-12): a Hubtask query line, names
-- already identifiers, read at each run as the rule's run_as. Empty is every entry the trigger
-- reaches, which is what every rule written before meant.
--
-- Expand only (rule 12): a text with a constant default writes no row. A pod of the previous
-- release never reads the column, and a rule it saves keeps the line it had.
-- +goose Up
ALTER TABLE automation_rule ADD COLUMN IF NOT EXISTS entries text NOT NULL DEFAULT '';

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
