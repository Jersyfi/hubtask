-- A data subject request's deadline is extended once (Art. 12(3), data-protection.md §4.1).
--
-- `due_at` stays the deadline in force and takes the extended date, so every reader of it - the
-- register's order and cursor, the overdue count behind alert A-19, `dsr_open_idx` - follows the
-- extension unchanged. `original_due_at` keeps the deadline before it and is the "once" marker;
-- `extension_reason` and `informed_on` are why, and the day the controller told the person.
--
-- Expand only: three nullable columns. A pod of the previous release reads `due_at` as before and
-- writes none of the new columns, so a rolling update needs nothing of it (rule 12).
-- +goose Up
ALTER TABLE data_subject_request ADD COLUMN IF NOT EXISTS original_due_at timestamptz;
ALTER TABLE data_subject_request ADD COLUMN IF NOT EXISTS extension_reason text
  CHECK (extension_reason IN ('COMPLEXITY','NUMBER_OF_REQUESTS'));
ALTER TABLE data_subject_request ADD COLUMN IF NOT EXISTS informed_on date;
-- An extension is whole or absent: a moved deadline without its reason is not one the law allows.
ALTER TABLE data_subject_request ADD CONSTRAINT dsr_extension_whole_check
  CHECK (num_nulls(original_due_at, extension_reason, informed_on) IN (0, 3));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
