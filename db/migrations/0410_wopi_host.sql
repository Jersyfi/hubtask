-- The WOPI host (UC-MED-06, ADR-0104): a workspace's document server, the lock a document server
-- holds on a file while somebody edits it there, and a file's earlier versions.
--
-- `document_server` is the administration's one choice per workspace - which server and its
-- origin; no row is none. `media_lock` is the protocol's lock, one per file, with its expiry: an
-- expired row is no lock and is overwritten by the next Lock. `media_version` names, per saved
-- version, the media object that keeps the bytes the file had before that save: the file's own id
-- stays what entries, pages and boards reference, and an earlier version is an object of its own,
-- counted as referenced while the file lives (RecountMediaReferences), swept by the ordinary
-- reconciliation once the file is gone. The access tokens are signed, never stored.
--
-- Expand only: new tables nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS document_server (
  tenant_id  uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  kind       text NOT NULL CHECK (kind IN ('COLLABORA_ONLINE','ONLYOFFICE','EURO_OFFICE','MICROSOFT_365')),
  url        text NOT NULL CHECK (url LIKE 'https://%'),
  set_by     uuid,
  updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS media_lock (
  tenant_id  uuid NOT NULL,
  media_id   uuid NOT NULL,
  lock_id    text NOT NULL CHECK (length(lock_id) BETWEEN 1 AND 1024),
  expires_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, media_id),
  CONSTRAINT media_lock_object_fkey FOREIGN KEY (tenant_id, media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media_version (
  tenant_id        uuid NOT NULL,
  media_id         uuid NOT NULL,
  number           integer NOT NULL CHECK (number >= 1),
  earlier_media_id uuid NOT NULL,
  saved_by         uuid,
  saved_at         timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, media_id, number),
  CONSTRAINT media_version_object_fkey FOREIGN KEY (tenant_id, media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT media_version_earlier_fkey FOREIGN KEY (tenant_id, earlier_media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS media_version_earlier_idx ON media_version (tenant_id, earlier_media_id);

ALTER TABLE document_server ENABLE ROW LEVEL SECURITY;
ALTER TABLE document_server FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON document_server;
CREATE POLICY tenant_isolation ON document_server
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON document_server TO hubtask_app;

ALTER TABLE media_lock ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_lock FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON media_lock;
CREATE POLICY tenant_isolation ON media_lock
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON media_lock TO hubtask_app;

ALTER TABLE media_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_version FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON media_version;
CREATE POLICY tenant_isolation ON media_version
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON media_version TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
