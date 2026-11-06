-- An import that only plans (backup-restore.md §9): the job converts the file and decides every
-- record, and writes nothing but its own report. Expand only (rule 12): a constant default, so the
-- ALTER rewrites nothing, and an instance that does not know the column still inserts real runs.

-- +goose Up

ALTER TABLE import_run ADD COLUMN IF NOT EXISTS dry_run boolean NOT NULL DEFAULT false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
