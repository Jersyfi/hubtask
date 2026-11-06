-- A page's pictures and files (UC-DOC-14): `page_media` names the media objects a page holds,
-- written by AttachPageMedia once the application has judged the writer and the object (rule 2),
-- and by a wiki's import with the pictures and attachments its export carries (UC-IMP-05). It is
-- what lets a reader of the page read the object (GetMedia) and what the media reconciliation
-- counts, so a picture is never reclaimed while its page exists - the body may drop it and a
-- version bring it back. A trashed page keeps its rows; the page's removal takes them. Archived
-- with the page (tenant-export.md §6).
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS page_media (
  tenant_id uuid NOT NULL,
  page_id   uuid NOT NULL,
  media_id  uuid NOT NULL,
  PRIMARY KEY (tenant_id, page_id, media_id),
  CONSTRAINT page_media_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT page_media_object_fkey FOREIGN KEY (tenant_id, media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS page_media_object_idx ON page_media (tenant_id, media_id);

ALTER TABLE page_media ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_media FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_media;
CREATE POLICY tenant_isolation ON page_media
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON page_media TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
