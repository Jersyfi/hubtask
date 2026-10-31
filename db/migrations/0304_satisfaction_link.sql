-- The satisfaction question by mail for a requester without an account (UC-SVC-07 check 6): a
-- link that answers it once, without signing in.
--
-- One link per requester and entry, hung on the contact that says the entry was filed for them:
-- a resolution mailed again replaces the token, and the requester's expiry (REQUESTER), an erasure
-- by address and the entry's deletion take the link with the contact by cascade. The token is
-- never stored - its keyed hash is, unique across the installation, found inside the tenant the
-- token names (public_form's way, migration 0302). Not archived: a live credential for a month.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS satisfaction_link (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  requester_id uuid NOT NULL,
  item_id      uuid NOT NULL,
  token_hash   bytea NOT NULL,
  created_at   timestamptz NOT NULL,
  expires_at   timestamptz NOT NULL,
  used_at      timestamptz,
  PRIMARY KEY (tenant_id, requester_id, item_id),
  CHECK (expires_at > created_at),
  CONSTRAINT satisfaction_link_contact_fkey FOREIGN KEY (tenant_id, requester_id, item_id)
    REFERENCES requester_contact (tenant_id, requester_id, item_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS satisfaction_link_token_uq ON satisfaction_link (token_hash);

ALTER TABLE satisfaction_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE satisfaction_link FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON satisfaction_link;
CREATE POLICY tenant_isolation ON satisfaction_link
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON satisfaction_link TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
