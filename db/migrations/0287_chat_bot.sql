-- A Matrix account in one room for one hub that speaks back (UC-INT-18): links to the hub's
-- entries answered with their key, title and status, `!task` messages made entries.
--
-- `chat_bot` keeps the homeserver's origin, the room, the account's own identifier and its access
-- token sealed under the installation's key, who it reads and writes as, and the sync position
-- the next run reads on from. Nothing a room says is stored: a captured title becomes an entry,
-- the rest is read and let go.
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS chat_bot (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  hub_id        uuid NOT NULL,
  collection_id uuid NOT NULL,
  homeserver    text NOT NULL CHECK (char_length(homeserver) <= 512 AND homeserver LIKE 'https://%'),
  room          text NOT NULL CHECK (char_length(room) BETWEEN 3 AND 255),
  bot_user      text NOT NULL CHECK (char_length(bot_user) <= 255),
  locale        text NOT NULL DEFAULT '' CHECK (char_length(locale) <= 16),
  unfurl        boolean NOT NULL,
  capture       boolean NOT NULL,
  enabled       boolean NOT NULL,
  run_as        uuid NOT NULL,
  since         text NOT NULL DEFAULT '' CHECK (char_length(since) <= 1024),
  failure       text NOT NULL DEFAULT '' CHECK (char_length(failure) <= 120),
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  created_at    timestamptz NOT NULL,
  updated_at    timestamptz NOT NULL,
  version       integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, hub_id, homeserver, room),
  CONSTRAINT chat_bot_run_as_fkey FOREIGN KEY (tenant_id, run_as)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT chat_bot_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT chat_bot_collection_fkey FOREIGN KEY (tenant_id, collection_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS chat_bot_hub_idx ON chat_bot (tenant_id, hub_id, created_at);

ALTER TABLE chat_bot ENABLE ROW LEVEL SECURITY;
ALTER TABLE chat_bot FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON chat_bot;
CREATE POLICY tenant_isolation ON chat_bot
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON chat_bot TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
