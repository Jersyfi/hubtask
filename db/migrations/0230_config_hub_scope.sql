-- A hub's own configuration document (config-document.md §1): a version and a proposal name the
-- hub they were applied to or planned for, NULL for the workspace's document. Expand only
-- (versioning-release.md §4): two nullable columns, nothing an older build reads changes. One
-- numbering per workspace stays - every scope's version is a version of the workspace - so the
-- primary key is unchanged.
--
-- The hub is a bare identifier rather than a foreign key, as the actors are: a version is
-- evidence and outlives the hub it configured.

-- +goose Up
ALTER TABLE config_revision ADD COLUMN IF NOT EXISTS hub_id uuid;
ALTER TABLE config_proposal ADD COLUMN IF NOT EXISTS hub_id uuid;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
