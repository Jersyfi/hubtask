-- Terms of use, agreed and re-agreed (UC-ID-19, identity.md §4.2 and §12).
--
-- `account.terms_url`, `terms_version` and `terms_agreed_at` are the person's latest agreement: the
-- link and the version of the terms in force when they agreed, and when. The agreement is current
-- while both equal the terms in force; every agreement is also in the workspace's trail
-- (`auth.terms_agreed`), so the row needs to hold only the latest.
--
-- `auth_pending` gains the purpose TERMS - the step a sign-in ends in while the account has not
-- agreed to the terms in force - and the session that step opens once agreed: `session_method` is
-- how the sign-in proved the account, `session_via` the provider that did, so the session at the end
-- of the step is the one the sign-in would have opened.
--
-- `oidc_flow.terms_url` and `terms_version` are the agreement a person gave on the invitation card
-- before choosing a provider; the arrival that activates the invited account stores it.
--
-- Expand only: nullable columns and a widened check. A process of the previous release neither
-- reads nor writes them and opens its sessions without the step (rule 12).
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS terms_url text
  CONSTRAINT account_terms_url_check CHECK (char_length(terms_url) <= 2000);
ALTER TABLE account ADD COLUMN IF NOT EXISTS terms_version text
  CONSTRAINT account_terms_version_check CHECK (char_length(terms_version) <= 64);
ALTER TABLE account ADD COLUMN IF NOT EXISTS terms_agreed_at timestamptz;

ALTER TABLE auth_pending DROP CONSTRAINT IF EXISTS auth_pending_purpose_check;
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_purpose_check
  CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT', 'ADDRESS', 'TERMS'));
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS session_method text
  CONSTRAINT auth_pending_session_method_check CHECK (char_length(session_method) <= 32);
ALTER TABLE auth_pending ADD COLUMN IF NOT EXISTS session_via uuid;

ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS terms_url text
  CONSTRAINT oidc_flow_terms_url_check CHECK (char_length(terms_url) <= 2000);
ALTER TABLE oidc_flow ADD COLUMN IF NOT EXISTS terms_version text
  CONSTRAINT oidc_flow_terms_version_check CHECK (char_length(terms_version) <= 64);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
