-- The LDAP directory a workspace synchronises its people and groups from (accepted.md A, UC-ID-31,
-- identity.md §27): one per workspace, its address (ldaps, or ldap upgraded by StartTLS - never a
-- plain bind), the account it reads as with the password sealed under the installation's key and
-- bound to the workspace (purpose "ldap.bind_password:<tenant>"), what it reads, the service
-- account the synchronisation acts as, and the last run's state for the administrators. Who the
-- directory holds is the `ldap:` prefix on account.external_id and account_group.external_id
-- (migration 0256), not a column here.
--
-- Expand only (rule 12): a new table nothing of the previous release reads or writes.

-- +goose Up
CREATE TABLE IF NOT EXISTS ldap_directory (
  tenant_id            uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  url                  text NOT NULL CHECK (url ~ '^ldaps?://' AND char_length(url) <= 500),
  bind_dn              text NOT NULL CHECK (char_length(bind_dn) BETWEEN 1 AND 1000),
  password_enc         bytea NOT NULL,
  password_key_id      text NOT NULL,
  user_base            text NOT NULL CHECK (char_length(user_base) BETWEEN 1 AND 1000),
  user_filter          text NOT NULL CHECK (char_length(user_filter) BETWEEN 1 AND 2000),
  group_base           text CHECK (group_base IS NULL OR char_length(group_base) BETWEEN 1 AND 1000),
  group_filter         text NOT NULL CHECK (char_length(group_filter) BETWEEN 1 AND 2000),
  id_attribute         text NOT NULL CHECK (char_length(id_attribute) BETWEEN 1 AND 100),
  mail_attribute       text NOT NULL CHECK (char_length(mail_attribute) BETWEEN 1 AND 100),
  name_attribute       text NOT NULL CHECK (char_length(name_attribute) BETWEEN 1 AND 100),
  group_name_attribute text NOT NULL CHECK (char_length(group_name_attribute) BETWEEN 1 AND 100),
  member_attribute     text NOT NULL CHECK (char_length(member_attribute) BETWEEN 1 AND 100),
  run_as               uuid NOT NULL,
  every_minutes        integer NOT NULL CHECK (every_minutes BETWEEN 15 AND 1440),
  enabled              boolean NOT NULL,
  next_run_at          timestamptz,
  last_run_at          timestamptz,
  last_outcome         text CHECK (last_outcome IN ('OK', 'PARTIAL', 'REFUSED', 'FAILED')),
  last_error           text CHECK (char_length(last_error) <= 200),
  last_counts          jsonb,
  failures             integer NOT NULL DEFAULT 0 CHECK (failures >= 0),
  created_at           timestamptz NOT NULL,
  updated_at           timestamptz NOT NULL,
  updated_by           uuid
);

ALTER TABLE ldap_directory ENABLE ROW LEVEL SECURITY;
ALTER TABLE ldap_directory FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ldap_directory
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON ldap_directory TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
