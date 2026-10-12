-- A managed account takes an address later (ADR-0074 §6, identity.md §13).
--
-- The address waits on a pending credential of its own purpose, ADDRESS, until somebody holding the
-- mailbox follows the link: nothing on the account changes before that, and an address nobody
-- confirmed leaves with the row when the session sweep deletes it after its day. The token is
-- minted when the mail goes out (the reset's discipline: the queue's payload carries identifiers
-- only), so an ADDRESS row holds no token hash until then - `token_hash` loses its NOT NULL, and a
-- NULL never matches a presented token.
--
-- Expand only: a nullable column, a dropped NOT NULL, a widened check and a check every existing
-- row meets. A pod of the previous release writes every column it knows and never an ADDRESS row
-- (rule 12).
-- +goose Up
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS address text
  CONSTRAINT auth_pending_address_length CHECK (char_length(address) BETWEEN 3 AND 254);
ALTER TABLE auth_pending ALTER COLUMN token_hash DROP NOT NULL;

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT', 'ADDRESS'));

-- An address belongs to an ADDRESS row and to nothing else; every other row carries a token from
-- the moment it exists.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_address_purpose;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_address_purpose
  CHECK ((purpose = 'ADDRESS') = (address IS NOT NULL)
         AND (purpose = 'ADDRESS' OR token_hash IS NOT NULL));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
