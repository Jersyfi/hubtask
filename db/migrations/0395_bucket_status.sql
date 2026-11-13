-- A board's columns show statuses (domain-model.md §3.5a, the owner's decision of 2026-11-12):
-- under a workflow each column maps one or more statuses, a status at most one column of the
-- board; a status no column shows stands in the virtual column *Ohne Spalte*. Without a workflow
-- the table holds nothing for the board.
--
-- Expand only (versioning-release.md §4): one new table, which no binary of before reads. The
-- primary key is the rule "one column per status and board"; the column's own deletion takes its
-- rows with it.

-- +goose Up
CREATE TABLE IF NOT EXISTS bucket_status (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id uuid NOT NULL,
  bucket_id     uuid NOT NULL,
  status        text NOT NULL CHECK (status ~ '^[a-z][a-z0-9_]{0,49}$'),
  PRIMARY KEY (tenant_id, collection_id, status),
  CONSTRAINT bucket_status_collection_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT bucket_status_bucket_fkey
    FOREIGN KEY (tenant_id, bucket_id) REFERENCES bucket (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS bucket_status_bucket_idx ON bucket_status (tenant_id, bucket_id);

ALTER TABLE bucket_status ENABLE ROW LEVEL SECURITY;
ALTER TABLE bucket_status FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON bucket_status
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON bucket_status TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
