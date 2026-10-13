-- A newcomer through a provider is created only once the terms of use are agreed (UC-ID-19,
-- identity.md §4.2).
--
-- Before this, an arrival the provider was allowed to create an account for was inserted, linked and
-- given the newcomer's access before the TERMS step; a person who declined kept an account they
-- never agreed to. Now, where terms are in force, the arrival waits on the TERMS row instead: which
-- provider and subject (`link_provider_id`, `link_subject`, as the LINK step carries them), and the
-- address, name and directory the provider vouched for. Agreeing creates the account from them.
--
-- Such a row has no account yet, so `account_id` becomes nullable. `address` (0131) cannot carry the
-- arrival's address: `auth_pending_address_purpose` holds it to purpose ADDRESS.
--
-- The arrival's columns are personal data by the data catalogue's reading. They live as long as the
-- row - minutes - and go with it: expired rows are swept by `DeleteExpiredPending`, the provider's
-- removal cascades, and no archive carries `auth_pending`.
--
-- Expand only. The previous release writes every row with an account and none of these columns, so
-- it passes both checks; it reads pending rows joined to their account, so a row without one is to
-- it an unknown token, refused as such (rule 12).
-- +goose Up
ALTER TABLE auth_pending ALTER COLUMN account_id DROP NOT NULL;

ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS arrival_email text
  CONSTRAINT auth_pending_arrival_email_length CHECK (char_length(arrival_email) BETWEEN 3 AND 254);
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS arrival_display_name text
  CONSTRAINT auth_pending_arrival_display_name_length CHECK (char_length(arrival_display_name) <= 200);
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS arrival_directory text
  CONSTRAINT auth_pending_arrival_directory_length CHECK (char_length(arrival_directory) <= 255);

-- An arrival exactly where there is no account yet.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_arrival_whole;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_arrival_whole
  CHECK ((account_id IS NULL) = (arrival_email IS NOT NULL));

-- A row without an account is a TERMS step carrying a provider's arrival, and nothing else.
ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_arrival_terms;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_arrival_terms
  CHECK (account_id IS NOT NULL OR (purpose = 'TERMS' AND link_provider_id IS NOT NULL));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
