-- A sign-in flow can carry the invitation it accepts (ADR-0078 §1, SC-32).
--
-- A provider's word alone does not activate an invited account: the second proof is the provider
-- being authoritative for the address, or the person arriving through the invitation's own link.
-- The link's token is checked when the flow starts - without spending it - and what the callback
-- needs of it is which account it invites, so that is what the flow keeps. The token itself never
-- travels to the provider and is not stored a second time.
--
-- Nullable, so the previous binary keeps writing the flows it knows; a flow without an invitation
-- is every sign-in. The key is the tenant-scoped one every reference to an account carries, and a
-- flow dies with the account it would have accepted.
-- +goose Up
ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS invited_account_id uuid;
ALTER TABLE oidc_flow DROP CONSTRAINT IF EXISTS oidc_flow_invited_account_fkey;
ALTER TABLE oidc_flow ADD CONSTRAINT oidc_flow_invited_account_fkey
  FOREIGN KEY (tenant_id, invited_account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
