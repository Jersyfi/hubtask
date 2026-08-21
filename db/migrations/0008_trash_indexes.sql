-- The indices the trash reads through (B-10).
--
-- Two shapes appear with this task, and neither is served by anything that exists.
--
-- The first is "give me back the batch": a container's deletion takes its subtree with it under one
-- identifier, and restoring it is one statement keyed on `trash_batch_id` (I-C2). Without an index
-- that is a sequential scan over every item in the tenant to find the handful a restore touches -
-- on the one operation where a person is waiting to see their work reappear.
--
-- The second is the trash view itself, ordered by when things were deleted. `wi_trash_idx`
-- (tenant_id, deleted_at) WHERE deleted_at IS NOT NULL already serves that for items; `container`
-- has no counterpart, because every index it carries is partial on `deleted_at IS NULL` - they
-- describe the live tree, and the trash is precisely what they exclude.
--
-- All three are partial on the column being non-NULL. The live tree is the overwhelming majority of
-- both tables and none of it belongs in an index about deletions: partial keeps them the size of the
-- trash rather than the size of the installation, and keeps a write to a live row out of them
-- entirely.
--
-- `tenant_id` leads even though row level security supplies it and no query names it. The planner
-- still sees the policy's predicate, and an index whose leading column is a batch identifier would
-- be scanned across tenants before that predicate narrowed it.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): CONCURRENTLY, so no
-- write is blocked while they build, which is why this migration takes no transaction. IF NOT EXISTS
-- covers the failure mode CONCURRENTLY has - an interrupted build leaves an invalid index behind,
-- and the retry must not then fail on the name.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_trash_batch_idx
  ON work_item (tenant_id, trash_batch_id)
  WHERE trash_batch_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS container_trash_batch_idx
  ON container (tenant_id, trash_batch_id)
  WHERE trash_batch_id IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS container_trash_idx
  ON container (tenant_id, deleted_at)
  WHERE deleted_at IS NOT NULL;

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS container_trash_idx;
DROP INDEX CONCURRENTLY IF EXISTS container_trash_batch_idx;
DROP INDEX CONCURRENTLY IF EXISTS wi_trash_batch_idx;
