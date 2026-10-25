-- Connected mailboxes (ADR-0094 decision 1): where a workspace's mail intake reads from when there
-- is no receiving domain, or beside it. The password, app password or token is sealed under the
-- installation's key and bound to the row (purpose "mailbox.secret:<id>"); nothing answers it back.
-- The read's state - when, and what it last failed with - is on the row for the administrators.

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
  created_at    timestamptz NOT NULL,
  created_by    uuid,
  UNIQUE (tenant_id, id),
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

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
