-- A provider names the directories it admits, beside the domains (ADR-0071 §2).
--
-- `allowed_email_domains` admits on the **text of an address**, and both providers this product has
-- a preset for say in their own documentation that this is the wrong check. Google: "The domain of
-- the email claim is insufficient to ensure that the account is managed by a domain or organization
-- — you must verify the `hd` claim explicitly." Microsoft: the `email` claim "isn't guaranteed to
-- be correct and is mutable over time - never use it for authorization".
--
-- A directory is the other thing: `tid` at Microsoft, `hd` at Google — the provider's own
-- identifier for the organisation a person belongs to, which it vouches for rather than reports.
-- A workspace that wrote `acme.example` into the domains list believed it admitted its own staff;
-- what it admitted was anybody at that provider who can verify an address in that domain.
--
-- **Both columns stay.** A `GENERIC` issuer has no directory claim and never will — for a
-- self-hosted Keycloak the address domain is all the token offers — so the domains list keeps that
-- job rather than being migrated away from.
--
-- **Nothing is rewritten.** A `MICROSOFT` or `GOOGLE` row that has domains and no directories
-- admits nobody under `DOMAINS` from this migration onwards, and the screen says why. Guessing a
-- directory from a domain is the one direction that hands out accounts: `acme.example` in the
-- domains list does not tell this database whether acme.example is a Workspace domain, a directory
-- GUID nobody typed, or a domain somebody's personal account happens to verify.

-- +goose Up

ALTER TABLE identity_provider
  ADD COLUMN allowed_directories text[] NOT NULL DEFAULT ARRAY[]::text[];

COMMENT ON COLUMN identity_provider.allowed_directories IS
  'The provider''s own identifiers for the organisations this row admits: Microsoft tid, Google hd. '
  'Empty under DOMAINS with a preset that has a directory claim admits nobody (ADR-0071 §2).';

-- +goose Down

ALTER TABLE identity_provider DROP COLUMN allowed_directories;
