-- The indexes the /views list runs on (D-07).
--
-- Until this milestone nothing read saved_view, so the table carried only its primary key and the
-- (tenant_id, id) unique index the composite foreign keys need. The list has two shapes -
-- "my views" by owner, and "what is shared along this container's path" by scope - and each gets
-- the index its predicate begins with. The reach index is partial: a PRIVATE view is only ever
-- found through its owner, so indexing its scope would index rows no query reads.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the indexes are built
-- CONCURRENTLY, so no write is blocked while they are built, which is also why this file runs
-- outside a transaction. The same operational caveat as migration 0004: an interrupted
-- CONCURRENTLY build leaves an invalid index that IF NOT EXISTS would skip - drop it and run
-- again.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS saved_view_owner_idx
  ON saved_view (tenant_id, owner_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS saved_view_reach_idx
  ON saved_view (tenant_id, scope_type, scope_id)
  WHERE sharing = 'SCOPE';
