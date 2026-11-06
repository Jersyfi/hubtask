-- How many workspaces' SAML sign-in is about to stop, across the installation (identity.md §26.1).
--
-- Each workspace's administrators are told 30, 7 and 1 days before their SAML provider's last
-- certificate ends; the operator saw nothing, because nothing may read identity providers across
-- workspaces and the end lives in a PEM certificate SQL cannot read. The certificate watch now
-- keeps that end on the row (`saml_certificates_end`, written when it changes, so a provider is
-- counted from its watch's next pass - within a day), and `saml_certificates_ending()`, a door of
-- `instance_census`'s shape, counts the enabled SAML providers whose end is known, those ending
-- within 30 and 7 days and those ended. Numbers only, never a provider or a workspace. The owner
-- already reads `identity_provider` by 0159's `doors_read` policy; `hubtask_app` gains nothing but
-- the function.
--
-- Expand only and safe for a rolling update: a nullable column and a function; an older binary
-- neither writes nor calls them.

-- +goose Up
ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS saml_certificates_end timestamptz;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION saml_certificates_ending()
RETURNS TABLE (providers bigint, within_30_days bigint, within_7_days bigint, ended bigint)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*),
         count(*) FILTER (WHERE saml_certificates_end > now() AND saml_certificates_end <= now() + interval '30 days'),
         count(*) FILTER (WHERE saml_certificates_end > now() AND saml_certificates_end <= now() + interval '7 days'),
         count(*) FILTER (WHERE saml_certificates_end <= now())
  FROM identity_provider
  WHERE kind = 'SAML' AND enabled AND saml_certificates_end IS NOT NULL
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION saml_certificates_ending() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION saml_certificates_ending() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
