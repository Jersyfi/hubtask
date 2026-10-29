-- A board's pictures and files (UC-CNV-08): `whiteboard_media` names the media objects placed on a
-- board, written by AttachWhiteboardMedia once the application has judged the writer and the
-- object (rule 2). It is what lets a reader of the board read the object (GetMedia) and what the
-- media reconciliation counts, so a picture is never reclaimed while its board exists - the scene
-- may drop it and a version bring it back. A trashed board keeps its rows; the board's removal
-- takes them. Archived with the board (tenant-export.md §6).
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS whiteboard_media (
  tenant_id     uuid NOT NULL,
  whiteboard_id uuid NOT NULL,
  media_id      uuid NOT NULL,
  PRIMARY KEY (tenant_id, whiteboard_id, media_id),
  CONSTRAINT whiteboard_media_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT whiteboard_media_object_fkey FOREIGN KEY (tenant_id, media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS whiteboard_media_object_idx ON whiteboard_media (tenant_id, media_id);

ALTER TABLE whiteboard_media ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_media FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard_media;
CREATE POLICY tenant_isolation ON whiteboard_media
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON whiteboard_media TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
