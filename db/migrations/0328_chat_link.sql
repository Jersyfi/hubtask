-- A person's chat account linked to a hub's chat bot (UC-SVC-09 check 6): the bot then tells
-- them of an approval waiting for their yes and takes their `!approve` or `!decline` as theirs.
--
-- A link begins pending: the person asks the app for a one-time code - only its SHA-256 is kept,
-- for fifteen minutes - and says `!link <code>` to the bot. The bot then keeps the platform's own
-- identifier of whoever said it (`chat_user`) and where a card to them goes (`address`: a Slack
-- user, a Teams personal conversation, a Mattermost `@name`, the Matrix room). One account per
-- chat user and bot, one chat user per account and bot. A bot or an account gone takes its links
-- with it.
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS chat_link (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  bot_id          uuid NOT NULL,
  account_id      uuid NOT NULL,
  chat_user       text NOT NULL DEFAULT '' CHECK (char_length(chat_user) <= 255),
  address         text NOT NULL DEFAULT '' CHECK (char_length(address) <= 1024),
  code_hash       bytea,
  code_expires_at timestamptz,
  created_at      timestamptz NOT NULL,
  linked_at       timestamptz,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, bot_id, account_id),
  CONSTRAINT chat_link_bot_fkey FOREIGN KEY (tenant_id, bot_id)
    REFERENCES chat_bot (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT chat_link_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS chat_link_user_idx ON chat_link (tenant_id, bot_id, chat_user) WHERE chat_user <> '';
CREATE UNIQUE INDEX IF NOT EXISTS chat_link_code_idx ON chat_link (tenant_id, bot_id, code_hash) WHERE code_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS chat_link_account_idx ON chat_link (tenant_id, account_id);

ALTER TABLE chat_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE chat_link FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON chat_link;
CREATE POLICY tenant_isolation ON chat_link
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON chat_link TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
