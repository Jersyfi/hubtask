-- A file in a store on an entry (UC-INT-14): a Nextcloud, SharePoint, OneDrive or WebDAV file or
-- folder linked by its address, never copied.
--
-- `file_link` is one per entry and address. Its address and name are content - a file's name says
-- what it holds - stored and never logged (rule 10).
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS file_link (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL,
  item_id    uuid NOT NULL,
  provider   text NOT NULL CHECK (provider IN ('NEXTCLOUD', 'SHAREPOINT', 'ONEDRIVE', 'WEBDAV')),
  address    text NOT NULL CHECK (char_length(address) BETWEEN 9 AND 2048 AND address LIKE 'https://%'),
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  added_by   uuid NOT NULL,
  added_at   timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, item_id, address),
  CONSTRAINT file_link_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS file_link_item_idx ON file_link (tenant_id, item_id, added_at);

ALTER TABLE file_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_link FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON file_link;
CREATE POLICY tenant_isolation ON file_link
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

-- DELETE is a link taken off its entry: the file itself stays in its store.
GRANT SELECT, INSERT, UPDATE, DELETE ON file_link TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
