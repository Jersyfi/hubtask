-- An import from another system (P-08, backup-restore.md §9).
--
-- A file somebody exported elsewhere - a CSV, a Trello board, a Google Tasks export, a Microsoft
-- To Do dump - is converted into the records a backup archive holds and applied through the
-- restore in MERGE mode with skip. This row is the import's own record: what was asked, where it
-- stands, and the report the applier wrote, in the restore's shape. Not a restore_run: that row
-- names a backup target and an archive at it, and an import has neither - the file is a media
-- object that is deleted when the job ends.
--
-- The mapping is the CSV kind's: which column carries which field. The refused rows are the
-- converter's: the lines it could not read, by number and code, while the rest of the file
-- landed. Expand only: a new table, read and written by the new version alone.
-- +goose Up
CREATE TABLE IF NOT EXISTS import_run (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  requested_by  uuid NOT NULL,
  kind          text NOT NULL CHECK (kind IN ('CSV','TRELLO','GOOGLE_TASKS','MICROSOFT_TODO')),
  media_id      uuid NOT NULL,
  hub_id        uuid NOT NULL,
  mapping       jsonb,
  -- The requester's zone and language at the request, for a source that writes dates without a
  -- zone and entries without a language: what the rows are read in.
  time_zone     text,
  language      text,
  status        text NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING','RUNNING','SUCCEEDED','FAILED')),
  report        jsonb,
  refused       jsonb,
  progress      jsonb,
  error_code    text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  started_at    timestamptz,
  finished_at   timestamptz
);
CREATE INDEX IF NOT EXISTS import_run_tenant_idx ON import_run (tenant_id, created_at DESC);
ALTER TABLE import_run ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_run FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON import_run
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON import_run TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
