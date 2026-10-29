-- A field hidden from readers (UC-ID-27 check 3). Expand only (versioning-release.md §4):
-- `custom_field_definition.read_restricted` leaves the value to holders of FIELD_READ:<key> or
-- FIELD_WRITE:<key>. Defaults to false, so nobody stops seeing anything; a build that predates
-- this one never reads the column. A hidden field is always restricted for writing as well.

-- +goose Up
ALTER TABLE custom_field_definition
  ADD COLUMN IF NOT EXISTS read_restricted boolean NOT NULL DEFAULT false;

ALTER TABLE custom_field_definition DROP CONSTRAINT IF EXISTS cfd_read_restricted_ck;
ALTER TABLE custom_field_definition
  ADD CONSTRAINT cfd_read_restricted_ck CHECK (NOT read_restricted OR restricted);

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
