-- When the workspace's owner last downloaded its whole archive (UC-BAK-07 check 7): the download
-- is allowed once in 24 hours, and this one value is what the window is judged by. Operational
-- state of the installation, not the workspace's data, so the archive leaves it out.
--
-- Expand only (rule 12): a nullable column without a default rewrites nothing, and a process of
-- the previous release neither writes nor reads it.

-- +goose Up
ALTER TABLE tenant ADD COLUMN IF NOT EXISTS archive_downloaded_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
