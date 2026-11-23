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
--
-- The previous release's media reconciliation does not count a picture: its recount writes zero and
-- a stamp, its pass then marks, its purge removes the row, and the cascade above takes the picture.
-- That process runs during a rolling update and after a rollback, so the database holds the line
-- itself: an object a picture names keeps a count of at least its pictures, no stamp and no mark,
-- whichever release writes it (`media_object_keeps_workspace_picture`). The set clears a stamp from
-- before with it. An object no picture names is untouched, so a replaced picture is reclaimed as
-- before; a delete - a replace-restore, a tenant's removal - is not an update and is not held.

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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION media_object_keeps_workspace_picture() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  pictures int;
BEGIN
  SELECT count(*) INTO pictures FROM workspace_picture w WHERE w.tenant_id = NEW.tenant_id AND w.media_id = NEW.id;
  IF pictures > 0 THEN
    NEW.ref_count := GREATEST(NEW.ref_count, pictures);
    NEW.unreferenced_since := NULL;
    IF OLD.deleted_at IS NULL THEN
      NEW.deleted_at := NULL;
    END IF;
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS media_object_keeps_workspace_picture ON media_object;
-- Only where a write would leave an object unreferenced, stamped or marked: the recount rewrites
-- every live row, and a lookup for each would be a second scan.
CREATE TRIGGER media_object_keeps_workspace_picture BEFORE UPDATE ON media_object
  FOR EACH ROW
  WHEN (NEW.ref_count < 1 OR NEW.unreferenced_since IS NOT NULL OR (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL))
  EXECUTE FUNCTION media_object_keeps_workspace_picture();

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
