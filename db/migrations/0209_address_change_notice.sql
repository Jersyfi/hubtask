-- A member without an email address is told of a change of the workspace's address at their next
-- sign-in (multi-tenancy.md §3.1, UC-INS-19 check 8).
--
-- The change writes one row per such member in its own transaction - the mail's counterpart - and
-- the member's acknowledgement removes it. One row per member: a second change before they signed in
-- replaces the first, keeping the address they last knew as `from_host`. Hosts only, never content.
--
-- Expand only: a new table nothing older reads (rule 12). Not in the archive: hosts belong to the
-- installation, and after a restore the notice is simply not told.

-- +goose Up
CREATE TABLE IF NOT EXISTS address_change_notice (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  from_host  text NOT NULL CHECK (length(from_host) BETWEEN 1 AND 253),
  to_host    text NOT NULL CHECK (length(to_host) BETWEEN 1 AND 253),
  changed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT address_change_notice_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE address_change_notice ENABLE ROW LEVEL SECURITY;
ALTER TABLE address_change_notice FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'address_change_notice' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON address_change_notice
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON address_change_notice TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
