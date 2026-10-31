-- Live documents across replicas (offline-sync.md §13, SY-E): an insert into a document's log rings
-- a channel of its own, `hubtask_document`, with the document's key, so that only that document's
-- connections catch up - before, every change of a workspace woke every open document of it, and
-- each asked its reader's permission again.
--
-- Expand only: the tenant's doorbell on `realtime_update` (0230) stays for a binary that listens
-- for it alone, during a rolling update.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_notify_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  -- At COMMIT, as hubtask_notify_change: a listener woken earlier would find nothing.
  PERFORM pg_notify('hubtask_document', NEW.tenant_id::text || '/' || NEW.kind || '/' || NEW.document_id::text);
  RETURN NULL;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS realtime_update_notify_document ON realtime_update;
CREATE TRIGGER realtime_update_notify_document
  AFTER INSERT ON realtime_update
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_document();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
