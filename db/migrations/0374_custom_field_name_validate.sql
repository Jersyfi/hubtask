-- 0362 added the custom field name's CHECK NOT VALID and nothing validated it, so the migrated
-- schema kept a constraint the restore drill and the migration test count as unproven. Validated
-- here without a transaction, under a lock writes pass; every row written since 0362 was held to it.

-- +goose NO TRANSACTION

-- +goose Up
ALTER TABLE custom_field_definition VALIDATE CONSTRAINT cfd_name_length;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
