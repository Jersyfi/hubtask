-- A flow bound to a session can be a connection rather than a step-up (ADR-0078 §1, SC-37).
--
-- A person signed in connects a provider identity from their own settings: a fresh sign-in at the
-- provider, bound to the session that asked, as a step-up at the provider is. What tells the two
-- apart is this flag. A connection's state finishes that connection and nothing else; the step-up's
-- consume and the sign-in's refuse it.
--
-- Additive, with a default, so the previous binary keeps writing the flows it knows. During a
-- rolling update the previous binary's step-up consume does not read the flag; a connection's state
-- presented there is judged as a step-up of the same session, whose proof demands the identity
-- already connected to the account - which a connection's new identity is not - so it proves
-- nothing and connects nothing.
-- +goose Up
ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS connect boolean NOT NULL DEFAULT false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
