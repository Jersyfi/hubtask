-- A template's collection part (configuration.md §6): the collection sections of a hub's document,
-- each block keyed without a handle, that a collection takes for itself. A collection that took it
-- records the version and that part in `config_hub_template_use` under its own identifier - the
-- column names a container - and a proposal names the collection it would record it for.
--
-- Expand only (rule 12): two nullable columns the previous release neither reads nor writes.
-- +goose Up
ALTER TABLE config_template ADD COLUMN IF NOT EXISTS collection_document jsonb;
ALTER TABLE config_proposal ADD COLUMN IF NOT EXISTS collection_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
