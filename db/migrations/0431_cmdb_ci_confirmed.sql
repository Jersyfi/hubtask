-- Who last confirmed an object is as recorded, and when (UC-AST-10): by the object's own act or by
-- a CONFIRMED answer in a campaign. last_seen_at is a source's sighting, not a person's word.
--
-- Expand only (rule 12): nullable columns nothing older reads or writes; an older binary's update
-- leaves them as they stand. confirmed_by is a bare identifier like updated_by.
-- +goose Up
ALTER TABLE cmdb_ci ADD COLUMN IF NOT EXISTS confirmed_at timestamptz;
ALTER TABLE cmdb_ci ADD COLUMN IF NOT EXISTS confirmed_by uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
