-- A page's or a whiteboard's live document goes with it (offline-sync.md §13: the owning kind
-- erases its documents with itself). Neither is removed on its own; both go by the cascade of the
-- hub they live in - a hub purged from the trash, a workspace purged, a restore replacing a
-- workspace - and `realtime_document` carries no foreign key to them, because its identifier is
-- the owner's of whichever kind. A row trigger on each owner fires on the cascade too, so no
-- path that removes a page or a board can leave its update log behind.
--
-- Expand only (rule 12): two functions and two triggers on deletes; the previous release deletes
-- no page or board outside these cascades, and erasing the log of one already gone changes no
-- answer it gives.

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION realtime_document_erased_with_page() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM realtime_document
  WHERE tenant_id = OLD.tenant_id AND kind = 'page' AND document_id = OLD.id;
  RETURN OLD;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION realtime_document_erased_with_whiteboard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM realtime_document
  WHERE tenant_id = OLD.tenant_id AND kind = 'whiteboard' AND document_id = OLD.id;
  RETURN OLD;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS page_realtime_document_erased ON page;
CREATE TRIGGER page_realtime_document_erased AFTER DELETE ON page
  FOR EACH ROW EXECUTE FUNCTION realtime_document_erased_with_page();

DROP TRIGGER IF EXISTS whiteboard_realtime_document_erased ON whiteboard;
CREATE TRIGGER whiteboard_realtime_document_erased AFTER DELETE ON whiteboard
  FOR EACH ROW EXECUTE FUNCTION realtime_document_erased_with_whiteboard();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
