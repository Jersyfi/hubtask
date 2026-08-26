-- The index the materialisation reads its own occurrences through (D-05).
--
-- Two questions run on it, both per series: whether an ON_COMPLETION series still has an open
-- entry, and when it was last completed. Without the index both are a scan of the tenant's
-- entries, on the path that runs every time somebody completes a recurring task.
--
-- Partial, because the column is NULL on every entry that belongs to no series - which is almost
-- all of them.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): an index changes no
-- row, it is built CONCURRENTLY so no write waits for it, and old code neither knows nor needs it.
-- An interrupted CONCURRENTLY build leaves an invalid index that IF NOT EXISTS would skip - drop it
-- and run again.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_recurrence_idx
  ON work_item (recurrence_rule_id)
  WHERE recurrence_rule_id IS NOT NULL;
