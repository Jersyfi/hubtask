-- A calendar feed pushed into its owner's calendar at Microsoft 365 or Google (UC-INT-16).
--
-- `calendar_push` is one per feed and provider: the provider account signed in with, the refresh
-- token sealed under the installation's key, and how the last run went. `calendar_push_event` is
-- what a push wrote for one entry - the provider's event and a digest of what it wrote - so a run
-- writes only what changed. `calendar_push_authorization` is a connection waiting ten minutes for
-- the provider's answer, its state stored as a hash and its PKCE verifier sealed.
--
-- Expand only: new tables nothing older reads (rule 12).
-- +goose Up
-- A feed's identity per workspace, which the push's tenant-scoped reference needs; a small table.
CREATE UNIQUE INDEX IF NOT EXISTS calendar_feed_tenant_id_uq ON calendar_feed (tenant_id, id);

CREATE TABLE IF NOT EXISTS calendar_push (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL,
  account_id        uuid NOT NULL,
  feed_id           uuid NOT NULL,
  provider          text NOT NULL CHECK (provider IN ('MICROSOFT', 'GOOGLE')),
  address           text NOT NULL DEFAULT '' CHECK (char_length(address) <= 320),
  secret_enc        bytea NOT NULL,
  secret_key_id     text NOT NULL,
  state             text NOT NULL CHECK (state IN ('ACTIVE', 'FAILED')),
  failure           text NOT NULL DEFAULT '' CHECK (char_length(failure) <= 120),
  pushed_at         timestamptz,
  created_at        timestamptz NOT NULL,
  updated_at        timestamptz NOT NULL,
  version           integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, feed_id, provider),
  CONSTRAINT calendar_push_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT calendar_push_feed_fkey FOREIGN KEY (tenant_id, feed_id)
    REFERENCES calendar_feed (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS calendar_push_account_idx ON calendar_push (tenant_id, account_id, created_at);

CREATE TABLE IF NOT EXISTS calendar_push_event (
  tenant_id  uuid NOT NULL,
  push_id    uuid NOT NULL,
  entry_id   uuid NOT NULL,
  remote_id  text NOT NULL CHECK (char_length(remote_id) BETWEEN 1 AND 1024),
  digest     text NOT NULL CHECK (char_length(digest) <= 64),
  pushed_at  timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, push_id, entry_id),
  CONSTRAINT calendar_push_event_push_fkey FOREIGN KEY (tenant_id, push_id)
    REFERENCES calendar_push (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS calendar_push_authorization (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  account_id      uuid NOT NULL,
  feed_id         uuid NOT NULL,
  provider        text NOT NULL CHECK (provider IN ('MICROSOFT', 'GOOGLE')),
  state_hash      bytea NOT NULL,
  verifier_enc    bytea NOT NULL,
  verifier_key_id text NOT NULL,
  expires_at      timestamptz NOT NULL,
  CONSTRAINT calendar_push_authorization_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS calendar_push_authorization_state_uq ON calendar_push_authorization (tenant_id, state_hash);

ALTER TABLE calendar_push ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_push FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON calendar_push;
CREATE POLICY tenant_isolation ON calendar_push
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON calendar_push TO hubtask_app;

ALTER TABLE calendar_push_event ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_push_event FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON calendar_push_event;
CREATE POLICY tenant_isolation ON calendar_push_event
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON calendar_push_event TO hubtask_app;

ALTER TABLE calendar_push_authorization ENABLE ROW LEVEL SECURITY;
ALTER TABLE calendar_push_authorization FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON calendar_push_authorization;
CREATE POLICY tenant_isolation ON calendar_push_authorization
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON calendar_push_authorization TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
