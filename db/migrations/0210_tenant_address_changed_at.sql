-- When a workspace's address last changed (multi-tenancy.md §3.1, UC-INS-19 check 3).
--
-- An owner may change the address only once the installation's cooldown has passed since the last
-- change, whoever made it; the screen names the day. Every change of address writes it in the
-- change's own transaction. A moment, never content.
--
-- Expand only: a nullable column nothing older reads or writes (rule 12). A workspace changed
-- before it existed counts as never changed. Not in the archive: after a restore the cooldown
-- simply starts over.

-- +goose Up
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS address_changed_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
