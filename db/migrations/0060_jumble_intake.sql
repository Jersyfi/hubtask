-- The jumble's webhook intake: a token-protected URL per tenant (G-10, automation.md §1.1's
-- credential discipline applied to the inbox).
--
-- One row per tenant, because there is exactly one address per tenant and rotating replaces it in
-- a single statement - the old token and the new one never both open the intake. Nothing here
-- stores the token: the hash is a value nobody can present, computed under the intake's own
-- purpose label, so a rule's inbound token presented here matches nothing and vice versa.

-- +goose Up

CREATE TABLE IF NOT EXISTS jumble_intake (
  tenant_id  uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL,
  -- When it was last minted, which is the only thing about the credential a read may show.
  rotated_at timestamptz NOT NULL
);

-- The lookup the unauthenticated route makes. The hash covers the whole presented string, tenant
-- half included, so a token rewritten to quote another tenant matches nothing at all.
CREATE UNIQUE INDEX IF NOT EXISTS jumble_intake_token_uq ON jumble_intake (token_hash);

-- Every tenant-scoped table is behind row level security, and a credential table least of all is
-- an exception.
ALTER TABLE jumble_intake ENABLE ROW LEVEL SECURITY;
ALTER TABLE jumble_intake FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON jumble_intake
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- The grant is explicit rather than left to the default privileges, because those follow the role
-- that creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON jumble_intake TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
