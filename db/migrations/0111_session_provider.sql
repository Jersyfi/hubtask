-- A session remembers which provider opened it (UC-ID-06 check 2, SC-09).
--
-- The session list says how each session was opened, and for one opened through a provider the
-- useful answer is the provider's name - "Contoso Entra ID" rather than "a provider". The method was
-- recorded since migration 0098; the provider was not.
--
-- No foreign key, on purpose. A provider removed is not a reason to touch the sessions it opened,
-- which end by their own bounds; the listing joins the provider under the reader's row policy and
-- names nothing for a row that is gone or not this tenant's to see. A key with ON DELETE SET NULL
-- would rewrite session rows of every workspace when the installation removes one of its own.
-- +goose Up
ALTER TABLE session ADD COLUMN IF NOT EXISTS signed_in_provider_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
