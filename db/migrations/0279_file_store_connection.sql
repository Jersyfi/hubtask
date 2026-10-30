-- A person's own sign-in to a file store, for picking a file on *Link a file* (UC-INT-17).
--
-- `file_store_connection` is a Nextcloud's app password the person granted by its login flow, or
-- a Microsoft refresh token for reading OneDrive and SharePoint, sealed under the installation's
-- key; the login is the store's name for the person. `file_store_authorization` is a connection
-- waiting ten minutes for the store's answer: its state stored as a hash, a PKCE verifier or a
-- login flow's poll token sealed.
--
-- Expand only: new tables nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS file_store_connection (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  account_id    uuid NOT NULL,
  kind          text NOT NULL CHECK (kind IN ('NEXTCLOUD', 'MICROSOFT')),
  server        text NOT NULL DEFAULT '' CHECK (char_length(server) <= 512 AND (server = '' OR server LIKE 'https://%')),
  login         text NOT NULL DEFAULT '' CHECK (char_length(login) <= 320),
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  created_at    timestamptz NOT NULL,
  used_at       timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, account_id, kind, server, login),
  CONSTRAINT file_store_connection_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS file_store_connection_account_idx ON file_store_connection (tenant_id, account_id, created_at);

CREATE TABLE IF NOT EXISTS file_store_authorization (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  account_id    uuid NOT NULL,
  kind          text NOT NULL CHECK (kind IN ('NEXTCLOUD', 'MICROSOFT')),
  server        text NOT NULL DEFAULT '' CHECK (char_length(server) <= 512),
  poll_endpoint text NOT NULL DEFAULT '' CHECK (char_length(poll_endpoint) <= 1024),
  state_hash    bytea NOT NULL,
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  expires_at    timestamptz NOT NULL,
  CONSTRAINT file_store_authorization_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS file_store_authorization_state_uq ON file_store_authorization (tenant_id, state_hash);

ALTER TABLE file_store_connection ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_store_connection FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON file_store_connection;
CREATE POLICY tenant_isolation ON file_store_connection
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON file_store_connection TO hubtask_app;

ALTER TABLE file_store_authorization ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_store_authorization FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON file_store_authorization;
CREATE POLICY tenant_isolation ON file_store_authorization
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON file_store_authorization TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
