-- The host a passkey was made for (identity.md §23.3).
--
-- A credential answers only at the relying party it was registered with. A workspace that moves to
-- another address, or an account restored into another workspace, keeps rows that can never sign
-- again - and counted as a second factor, such a row would let the authenticator app go and leave a
-- person whose workspace requires a factor with nothing their sign-in can ask for (P-16). So each row
-- records its RP ID, and only those of the workspace's current one count.
--
-- Expand only: a nullable column the previous release never writes, and a row without one - made
-- before it existed - counts as the workspace's (rule 12).

-- +goose Up
ALTER TABLE webauthn_credential ADD COLUMN IF NOT EXISTS rp_id text
  CHECK (rp_id IS NULL OR char_length(rp_id) BETWEEN 1 AND 253);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
