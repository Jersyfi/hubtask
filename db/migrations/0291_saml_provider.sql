-- A SAML 2.0 identity provider beside OpenID Connect (accepted.md A, identity.md §26): the kind
-- SAML, its single sign-on address and signing certificates, and on a sign-in flow the answer the
-- assertion consumer service kept for the callback that verifies it.
--
-- Expand only (rule 12): a widened CHECK and nullable columns an older binary neither reads nor
-- writes. An older binary meeting a SAML row reads a kind it does not know, which admits nobody.
-- The provider's and the flow's checks are added NOT VALID and validated in 0335, outside a
-- transaction; inline on a new column a check scans the table under the ALTER's exclusive lock.

-- +goose Up
ALTER TABLE identity_provider DROP CONSTRAINT IF EXISTS identity_provider_kind_check;
ALTER TABLE identity_provider
  ADD CONSTRAINT identity_provider_kind_check CHECK (kind IN ('GENERIC', 'GOOGLE', 'MICROSOFT', 'SAML')) NOT VALID,
  ADD COLUMN IF NOT EXISTS saml_sso_url     text,
  ADD COLUMN IF NOT EXISTS saml_certificate text;
ALTER TABLE identity_provider
  DROP CONSTRAINT IF EXISTS identity_provider_saml_sso_url_check,
  ADD CONSTRAINT identity_provider_saml_sso_url_check CHECK (length(saml_sso_url) <= 2000) NOT VALID,
  DROP CONSTRAINT IF EXISTS identity_provider_saml_certificate_check,
  ADD CONSTRAINT identity_provider_saml_certificate_check CHECK (length(saml_certificate) <= 20000) NOT VALID;
-- Both or neither, and only for SAML - and never the installation's: a SAML provider is a
-- workspace's own (identity.md §26).
ALTER TABLE identity_provider
  ADD CONSTRAINT identity_provider_saml_whole CHECK (
    (kind = 'SAML') = (saml_sso_url IS NOT NULL AND saml_certificate IS NOT NULL)
    AND (kind <> 'SAML' OR tenant_id IS NOT NULL)) NOT VALID;

-- The provider's answer, kept once by the assertion consumer service and verified only when the
-- callback spends the flow; the handoff code's hash binds that callback to the redirect the
-- service answered. Gone with the flow at the sweep.
ALTER TABLE oidc_flow
  ADD COLUMN IF NOT EXISTS saml_response     text,
  ADD COLUMN IF NOT EXISTS saml_handoff_hash bytea;
ALTER TABLE oidc_flow DROP CONSTRAINT IF EXISTS oidc_flow_saml_response_check,
  ADD CONSTRAINT oidc_flow_saml_response_check CHECK (length(saml_response) <= 350000) NOT VALID;
