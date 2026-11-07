-- The index behind the CIs an account owns (UC-SVC-03 check 9): the requester's assets beside a
-- request are read by owner across the workspace's hubs, which cmdb_ci_hub_idx cannot serve.
-- CONCURRENTLY and without a transaction, so no write waits while it builds (versioning-release.md
-- §4); IF NOT EXISTS for a retry after an interrupted build.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS cmdb_ci_owner_account_idx
  ON cmdb_ci (tenant_id, owner_account_id)
  WHERE owner_account_id IS NOT NULL AND merged_into IS NULL;

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
