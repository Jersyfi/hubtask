-- A directory provisions people and groups over SCIM 2.0 (UC-ID-28, identity.md §26): the
-- identifier the directory knows a person or a group by, beside the workspace's own. Unique per
-- workspace, so the directory finds again what it created; nullable, because everybody invited by
-- hand or made by a provider's first arrival has none.
--
-- Expand only (rule 12): two nullable columns nothing of the previous release writes, length
-- bounds every stored row satisfies (all NULL), added NOT VALID and then validated, and unique
-- indexes over columns that hold no value yet.
-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS external_id text;
ALTER TABLE account ADD CONSTRAINT account_external_id_length_check
  CHECK (external_id IS NULL OR char_length(external_id) BETWEEN 1 AND 255) NOT VALID;
ALTER TABLE account VALIDATE CONSTRAINT account_external_id_length_check;
CREATE UNIQUE INDEX IF NOT EXISTS account_external_id_uq
  ON account (tenant_id, external_id) WHERE external_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE account_group ADD COLUMN IF NOT EXISTS external_id text;
ALTER TABLE account_group ADD CONSTRAINT account_group_external_id_length_check
  CHECK (external_id IS NULL OR char_length(external_id) BETWEEN 1 AND 255) NOT VALID;
ALTER TABLE account_group VALIDATE CONSTRAINT account_group_external_id_length_check;
CREATE UNIQUE INDEX IF NOT EXISTS account_group_external_id_uq
  ON account_group (tenant_id, external_id) WHERE external_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
