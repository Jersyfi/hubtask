-- The configuration tables in the workspace archive (tenant-export.md §6). Expand only: a
-- restore that replaces a workspace empties each restored table first, so the application role
-- may delete the configuration versions, proposals, templates and template uses of its own
-- workspace. Row level security keeps that to the workspace the transaction is in (rule 3).

-- +goose Up
GRANT DELETE ON config_revision TO hubtask_app;
GRANT DELETE ON config_proposal TO hubtask_app;
GRANT DELETE ON config_template TO hubtask_app;
GRANT DELETE ON config_template_use TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
