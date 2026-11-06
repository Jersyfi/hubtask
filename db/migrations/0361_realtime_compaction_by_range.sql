-- Live documents compact by range rather than by marking (offline-sync.md §13, data-retention.md
-- §3). An update is compacted once a snapshot covers its place: `seq <= up_to_seq` of a snapshot,
-- since the snapshot's `created_at`. Marking each update `compacted_at` rewrote every row of the
-- log once before retention deleted it, and the partial index on the mark kept that rewrite from
-- being a HOT update - a dead tuple and an index entry per keystroke. From this release nothing
-- writes the mark; `compacted_at` and its index stay for the previous release's pods during the
-- rollout, and are dropped by a later contract step.
--
-- The head is updated on every append, so it keeps room on its page for a HOT update; the log and
-- the figures' touch table churn faster than autovacuum's default fifth of the table, so they are
-- vacuumed sooner.
--
-- Expand only (rule 12): storage parameters, which take a lock that writes pass.
-- +goose Up
ALTER TABLE realtime_document SET (fillfactor = 70);
ALTER TABLE realtime_update SET (autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.05);
ALTER TABLE collection_figure_touch SET (autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.05);

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
