-- An address the mail server refused for good (an SMTP 5xx at RCPT or DATA): the notification
-- queue stops sending to it until the person clears the mark (observability-reliability.md, mail).
-- Per account, with the address it was refused at, so a changed address is tried again at once;
-- it goes with the account.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS mail_undeliverable (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  address    text NOT NULL CHECK (char_length(address) <= 320),
  since      timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT mail_undeliverable_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE mail_undeliverable ENABLE ROW LEVEL SECURITY;
ALTER TABLE mail_undeliverable FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON mail_undeliverable;
CREATE POLICY tenant_isolation ON mail_undeliverable
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON mail_undeliverable TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
