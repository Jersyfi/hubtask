-- An issue tracker's projects and a wiki's space as import kinds (D-10, UC-IMP-04, UC-IMP-05).
--
-- Expand only (rule 12): the CHECK widens, added NOT VALID and then validated, and a binary of the
-- release before writes none of the new kinds - it refuses them by name before a run is written.
-- +goose Up
ALTER TABLE import_run DROP CONSTRAINT IF EXISTS import_run_kind_check;
ALTER TABLE import_run ADD CONSTRAINT import_run_kind_check
  CHECK (kind IN ('CSV','TRELLO','GOOGLE_TASKS','MICROSOFT_TODO','JIRA','CONFLUENCE')) NOT VALID;
ALTER TABLE import_run VALIDATE CONSTRAINT import_run_kind_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
