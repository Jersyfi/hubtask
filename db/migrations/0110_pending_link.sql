-- A pending credential may carry the provider identity it connects (ADR-0071's addendum, E2).
--
-- Before this, a provider arrival whose address matched an existing account was linked to it on the
-- provider's word, and the session opened without the account's second factor. Connecting now waits
-- for the account's own proof: the arrival receives a LINK credential, which completes with the
-- account's password and - where the account has a second factor - continues into the TOTP step. The
-- identity to connect has to survive both steps, and the pending row is the one thing both steps
-- hold, so it travels there: which provider, and which subject it vouched for.
--
-- `link_subject` is the provider's identifier for the person, personal data by the data catalogue's
-- reading. It lives as long as the row - minutes - and goes with it: consumed rows and expired ones
-- are swept by `DeleteExpiredPending`, and the account's deletion cascades.
--
-- The provider reference cascades too: a provider removed while somebody is mid-way through
-- connecting leaves nothing to connect, and the step then refuses as an unknown credential would.
-- +goose Up
ALTER TABLE auth_pending
  ADD COLUMN IF NOT EXISTS link_provider_id uuid REFERENCES identity_provider(id) ON DELETE CASCADE,
  ADD COLUMN IF NOT EXISTS link_subject text CHECK (length(link_subject) BETWEEN 1 AND 255);

-- Both or neither: half a link is a row nobody could act on.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_link_whole;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_link_whole
  CHECK ((link_provider_id IS NULL) = (link_subject IS NULL));

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK'));

-- A LINK credential exists only to connect something.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_link_purpose;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_link_purpose
  CHECK (purpose <> 'LINK' OR link_provider_id IS NOT NULL);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
