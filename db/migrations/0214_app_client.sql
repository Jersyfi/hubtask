-- The built-in client of the installed apps (identity.md §15.3, ADR-0096): every workspace knows it
-- without a registration. Each consent through it writes a client row of its own, marked with the
-- app's platform and named by the device, so that one person's apps on two devices are two grants
-- they sign out one by one, and the registry an administrator manages keeps only what was
-- registered.
--
-- Expand only and safe for a rolling update: one nullable column the previous release neither
-- writes nor reads, and checks that hold for every row it wrote.

-- +goose Up
ALTER TABLE oauth_client
  ADD COLUMN IF NOT EXISTS app_platform text
    CHECK (app_platform IN ('windows', 'linux', 'android'));

-- The apps keep no secret: a row of the built-in client is public, its whole authentication PKCE.
ALTER TABLE oauth_client
  ADD CONSTRAINT oauth_client_app_public CHECK (app_platform IS NULL OR NOT confidential);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
