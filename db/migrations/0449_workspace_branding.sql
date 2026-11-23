-- A workspace's own name and logo (UC-INS-22, ADR-0112): its three pictures, one row each, and
-- `WORKSPACE_LOGO`, the usage they are staged for. What the header shows is a key of the
-- workspace's settings (`header_shows`), which needs no column: a choice references nothing.
--
-- A table of its own rather than three keys on `tenant`: a restore writes the workspace's row first
-- and its media after it, so a key from that row to a picture could not be restored, and a reference
-- inside `tenant.settings` would have no key and no count in the media reconciliation, which would
-- remove the picture. Here each row names its picture by (tenant, media) and goes with it
-- (`ON DELETE CASCADE`): a replace-restore empties `media_object` before writing it back and leaves
-- the workspace without that picture rather than failing. An erasure of the uploader spares a
-- picture a row here names, as it spares an attachment.
--
-- Expand only and safe for a rolling update (rule 12): a new table a process of the previous release
-- neither reads nor writes, and a usage check that only widens - an older process never stages the
-- new usage. The widened check is added NOT VALID and validated in 0450, outside a transaction;
-- every existing row satisfies it, so the validation cannot fail.

-- +goose Up
CREATE TABLE IF NOT EXISTS workspace_picture (
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  picture   text NOT NULL CONSTRAINT workspace_picture_picture_check
            CHECK (picture IN ('LOGO_LIGHT', 'LOGO_DARK', 'SYMBOL')),
  media_id  uuid NOT NULL,
  PRIMARY KEY (tenant_id, picture),
  CONSTRAINT workspace_picture_media_id_fkey
    FOREIGN KEY (tenant_id, media_id) REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);
-- The media's cascade and the reconciliation's count find a picture's rows without a scan.
CREATE INDEX IF NOT EXISTS workspace_picture_media_idx ON workspace_picture (tenant_id, media_id);

ALTER TABLE workspace_picture ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspace_picture FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'workspace_picture' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON workspace_picture
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON workspace_picture TO hubtask_app;

ALTER TABLE media_object DROP CONSTRAINT IF EXISTS media_object_usage_check;
ALTER TABLE media_object ADD CONSTRAINT media_object_usage_check
  CHECK (usage IN ('COVER', 'ATTACHMENT', 'IMPORT', 'EXPORT', 'AVATAR', 'DESK_LOGO', 'WORKSPACE_LOGO')) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
