-- Connected mailboxes (ADR-0094 decision 1): where a workspace's mail intake reads from when there
-- is no receiving domain, or beside it. The password, app password or token is sealed under the
-- installation's key and bound to the row (purpose "mailbox.secret:<id>"); nothing answers it back.
-- The read's state - when, and what it last failed with - is on the row for the administrators.
-- A mailbox signs in with a password, or with OAuth (IMAP SASL XOAUTH2, JMAP bearer): then the
-- sealed secret is the refresh token (and the client secret of an app the administrator entered),
-- and the row names the provider's endpoints and the client. Any provider's, not only Google's and
-- Microsoft's. A connection waiting for the provider's answer is a mailbox_authorization: used
-- once, gone after ten minutes, bound to the administrator who asked.

-- +goose Up
CREATE TABLE IF NOT EXISTS mailbox (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  protocol      text NOT NULL CHECK (protocol IN ('IMAP', 'JMAP')),
  address       text NOT NULL,
  host          text,
  port          integer CHECK (port IS NULL OR port BETWEEN 1 AND 65535),
  session_url   text,
  username      text NOT NULL,
  folder        text NOT NULL,
  done_folder   text NOT NULL,
  preset        text,
  enabled       boolean NOT NULL DEFAULT true,
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  checked_at    timestamptz,
  failure       text,
  failing_since timestamptz,
  auth          text NOT NULL DEFAULT 'PASSWORD' CHECK (auth IN ('PASSWORD', 'OAUTH')),
  oauth_provider      text,
  oauth_authorize_url text,
  oauth_token_url     text,
  oauth_scopes        text[],
  oauth_client_id     text,
  oauth_installation  boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL,
  created_by    uuid,
  UNIQUE (tenant_id, id),
  CHECK ((auth = 'OAUTH') = (oauth_provider IS NOT NULL AND oauth_token_url IS NOT NULL)),
  CHECK ((protocol = 'IMAP') = (host IS NOT NULL AND port IS NOT NULL)),
  CHECK ((protocol = 'JMAP') = (session_url IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS mailbox_address_uq ON mailbox (tenant_id, lower(address));

ALTER TABLE mailbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE mailbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mailbox
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON mailbox TO hubtask_app;

CREATE TABLE IF NOT EXISTS mailbox_authorization (
  id                   uuid PRIMARY KEY,
  tenant_id            uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id           uuid NOT NULL,
  state_hash           bytea NOT NULL,
  provider             text NOT NULL,
  authorize_url        text NOT NULL,
  token_url            text NOT NULL,
  scopes               text[] NOT NULL,
  client_id            text NOT NULL,
  installation         boolean NOT NULL,
  host                 text NOT NULL,
  port                 integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
  login_hint           text,
  verifier_enc         bytea NOT NULL,
  verifier_key_id      text NOT NULL,
  client_secret_enc    bytea,
  client_secret_key_id text,
  expires_at           timestamptz NOT NULL,
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS mailbox_authorization_state_uq ON mailbox_authorization (tenant_id, state_hash);

ALTER TABLE mailbox_authorization ENABLE ROW LEVEL SECURITY;
ALTER TABLE mailbox_authorization FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mailbox_authorization
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON mailbox_authorization TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
