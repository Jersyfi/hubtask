-- A SAML 2.0 identity provider beside OpenID Connect (accepted.md A, identity.md §26): the kind
-- SAML, its single sign-on address and signing certificates, and on a sign-in flow the answer the
-- assertion consumer service kept for the callback that verifies it.
--
-- Expand only (rule 12): a widened CHECK and nullable columns an older binary neither reads nor
-- writes. An older binary meeting a SAML row reads a kind it does not know, which admits nobody.

-- +goose Up
ALTER TABLE identity_provider DROP CONSTRAINT IF EXISTS identity_provider_kind_check;
ALTER TABLE identity_provider
  ADD CONSTRAINT identity_provider_kind_check CHECK (kind IN ('GENERIC', 'GOOGLE', 'MICROSOFT', 'SAML')),
  ADD COLUMN IF NOT EXISTS saml_sso_url     text CHECK (length(saml_sso_url) <= 2000),
  ADD COLUMN IF NOT EXISTS saml_certificate text CHECK (length(saml_certificate) <= 20000);
-- Both or neither, and only for SAML - and never the installation's: a SAML provider is a
-- workspace's own (identity.md §26).
ALTER TABLE identity_provider
  ADD CONSTRAINT identity_provider_saml_whole CHECK (
    (kind = 'SAML') = (saml_sso_url IS NOT NULL AND saml_certificate IS NOT NULL)
    AND (kind <> 'SAML' OR tenant_id IS NOT NULL));

-- The provider's answer, kept once by the assertion consumer service and verified only when the
-- callback spends the flow; the handoff code's hash binds that callback to the redirect the
-- service answered. Gone with the flow at the sweep.
ALTER TABLE oidc_flow
  ADD COLUMN IF NOT EXISTS saml_response     text CHECK (length(saml_response) <= 350000),
  ADD COLUMN IF NOT EXISTS saml_handoff_hash bytea;
