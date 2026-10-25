-- The mail intake (ADR-0094): a workspace's settings, and a receipt per message it received.
--
-- One settings row per workspace, written by its administrators: on or off, its name on the
-- operator's receiving domain (unique on the installation, so an address names one workspace),
-- the default collection, who may write, and the rules that file a sender's mail. The receipts
-- make a redelivered message a no-op - one per workspace and Message-ID - and are the log the
-- administrators read; they go after 30 days.

-- +goose Up
CREATE TABLE IF NOT EXISTS mail_setting (
  tenant_id             uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  enabled               boolean NOT NULL DEFAULT false,
  name                  text CHECK (name IS NULL OR name ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$'),
  default_collection_id uuid,
  sender_level          text NOT NULL DEFAULT 'MEMBERS'
                        CHECK (sender_level IN ('MEMBERS', 'MEMBERS_AND_ALLOWED', 'ANYONE')),
  allowed               text[] NOT NULL DEFAULT '{}',
  blocked               text[] NOT NULL DEFAULT '{}',
  sender_routes         jsonb NOT NULL DEFAULT '[]'::jsonb,
  opened_by             uuid,
  updated_at            timestamptz NOT NULL,
  updated_by            uuid
);
CREATE UNIQUE INDEX IF NOT EXISTS mail_setting_name_uq ON mail_setting (name) WHERE name IS NOT NULL;

ALTER TABLE mail_setting ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_setting FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mail_setting
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE ON mail_setting TO hubtask_app;

-- +goose StatementBegin
-- resolve_mail_workspace is the workspace an address on the operator's domain names, before any
-- tenant is known - and only one whose intake is on: a switched-off workspace has no address.
CREATE OR REPLACE FUNCTION resolve_mail_workspace(workspace_name text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT tenant_id FROM mail_setting
  WHERE name = lower(workspace_name) AND enabled
  LIMIT 1
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION resolve_mail_workspace(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_mail_workspace(text) TO hubtask_app;

-- +goose StatementBegin
-- mail_workspace_name_taken answers whether another workspace holds the name, which RLS would hide.
CREATE OR REPLACE FUNCTION mail_workspace_name_taken(workspace_name text, self uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT EXISTS (SELECT 1 FROM mail_setting WHERE name = lower(workspace_name) AND tenant_id <> self)
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION mail_workspace_name_taken(text, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mail_workspace_name_taken(text, uuid) TO hubtask_app;

CREATE TABLE IF NOT EXISTS mail_receipt (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  message_id  text NOT NULL,
  received_at timestamptz NOT NULL,
  sender      text NOT NULL,
  subject     text NOT NULL,
  outcome     text NOT NULL CHECK (outcome IN ('ENTRY', 'COMMENT', 'JUMBLE', 'DISCARDED')),
  reason      text NOT NULL DEFAULT '',
  item_id     uuid,
  entry_id    uuid,
  door        text NOT NULL,
  UNIQUE (tenant_id, message_id)
);
CREATE INDEX IF NOT EXISTS mail_receipt_recent_idx ON mail_receipt (tenant_id, received_at DESC);

ALTER TABLE mail_receipt ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_receipt FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON mail_receipt
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON mail_receipt TO hubtask_app;

-- Why a message waits in the jumble, beside the place it was meant for (0201).
ALTER TABLE jumble_entry ADD COLUMN IF NOT EXISTS intake_reason text;

-- ADR-0092's random codes go: a collection's handle and an entry's ticket key name the place now,
-- and a notification's reply goes to the key on the intake's address.
DROP FUNCTION IF EXISTS resolve_mail_address(text);
DROP TABLE IF EXISTS mail_address;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
