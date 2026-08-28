-- The read the engine makes on every event, which had no index that serves it (G-07,
-- automation.md §2). The other one it makes - the throttle's count - already has one.

-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for CONCURRENTLY's interrupted-build failure mode (drop the invalid index and run again).

-- +goose NO TRANSACTION

-- +goose Up

-- What the subscriber asks per event: the enabled rules whose trigger is this type.
--
-- An expression index, because the trigger is a document rather than columns - it has been since
-- 0001_init, and the shape is right: a trigger's fields belong to its kind, and six kinds' worth of
-- nullable columns would be a table nobody can read. What the document costs is that a predicate on
-- it needs an index built on the same expressions, which is this.
--
-- `rule_trigger_idx` beside it stays. It answers "which rules does this tenant have enabled", which
-- is the listing's question rather than the dispatcher's, and a partial index on a different
-- predicate is not a duplicate of one on this.
--
-- Partial on enabled and not deleted, because a dispatcher never asks about any other rule - and a
-- workspace's disabled and deleted rules are exactly what should not cost an index entry each.
CREATE INDEX CONCURRENTLY IF NOT EXISTS rule_event_trigger_idx
  ON automation_rule (tenant_id, (trigger ->> 'kind'), (trigger ->> 'event_type'))
  WHERE deleted_at IS NULL AND enabled = true;

-- The throttle's count needs no index of its own: `rule_run_idx` is already
-- (tenant_id, rule_id, started_at DESC), which is exactly the read. Recorded here because the
-- absence is a decision - a second index on the same columns would cost every run a write and
-- answer nothing the first one does not.

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
