-- The indexes behind the sweep of ended timers (TIMER) and of connector runs (CONNECTOR_RUN,
-- data-retention.md §3): every SLA goal, OLA, approval level and page review arms a timer, and a
-- connector writes a run with its decisions every quarter of an hour at most - both tables only
-- grew. And the one behind a run's cascade to its held items, which scanned the tenant's holds per
-- deleted run. CONCURRENTLY and without a transaction, so no write waits while they build
-- (versioning-release.md §4); the migrator rebuilds one an interrupted build left invalid.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS durable_timer_ended_idx
  ON durable_timer (tenant_id, updated_at) WHERE state IN ('FIRED', 'STOPPED');
CREATE INDEX CONCURRENTLY IF NOT EXISTS cmdb_run_started_idx
  ON cmdb_run (tenant_id, started_at);
CREATE INDEX CONCURRENTLY IF NOT EXISTS cmdb_hold_run_idx
  ON cmdb_hold (tenant_id, run_id);

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
