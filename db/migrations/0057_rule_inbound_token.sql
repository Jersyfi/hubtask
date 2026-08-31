-- The address an INBOUND_WEBHOOK rule answers on (G-08, automation.md §1.1).
--
-- A token-protected URL per rule, with D-08's credential discipline: hashed with the installation
-- secret under its own purpose label, answered once when it is minted, and revoked by rotating.
-- Nothing here stores the token - the hash is a value nobody can present.
--
-- On the rule rather than in a table of its own, because there is exactly one address per rule and
-- the address *is* the rule's: a table would be a second identity for the same thing, and a rule
-- with two live addresses is what "revocable by rotating" exists to prevent.

-- Forward-only and safe for a rolling update: two nullable columns, which PostgreSQL adds without
-- rewriting the table, and an index built CONCURRENTLY outside a transaction.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE automation_rule
  ADD COLUMN IF NOT EXISTS inbound_token_hash bytea,
  -- When it was last minted, which is the only thing about the credential a listing may show. Not
  -- a prefix and not a masked value: a token that is partly readable is a token whose guessing
  -- space has been narrowed for whoever reads the listing.
  ADD COLUMN IF NOT EXISTS inbound_rotated_at timestamptz;

-- The lookup the unauthenticated route makes, and the reason the token names its own tenant: the
-- hash is unique across the installation, so a token rewritten to quote another tenant matches
-- nothing at all. Partial, because most rules have no address.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS automation_rule_inbound_token_uq
  ON automation_rule (inbound_token_hash)
  WHERE inbound_token_hash IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
