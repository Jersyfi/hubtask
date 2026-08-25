-- The bookkeeping that makes a scheduling announcement happen once (D-03).
--
-- item.due_soon and item.overdue are announced once per due date, which is not something a scan
-- can decide from the row alone: "has this entry's deadline already been announced" is a fact
-- about the announcement rather than about the deadline. Two stamps on the entry answer it, and
-- the writer of the due date clears them - a date that moves is a new deadline, which may be
-- approached and missed again.
--
-- On the entry rather than in a table of their own: they are one bit each about one row, they die
-- with it, and a table would be a join on the hottest path in the system for two timestamps.
--
-- The index is the scan's. wi_due_idx has collection_id between the tenant and the date, which
-- serves "what is due in this collection" and not "what does this tenant owe next" - the question
-- the scheduler asks. This one is partial twice over: only entries that are open, and only those
-- with something left to announce, which keeps it to the rows the scan can actually claim.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): both columns are
-- nullable with no default, so the table is not rewritten and every existing row reads as "not
-- announced yet" - which is the truth. Old code neither writes nor reads them. The index is built
-- CONCURRENTLY, so no write waits for it, which is why this file runs outside a transaction; an
-- interrupted build leaves an invalid index that IF NOT EXISTS would skip - drop it and run again.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE work_item ADD COLUMN IF NOT EXISTS due_soon_announced_at timestamptz;
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS overdue_announced_at timestamptz;

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_due_announce_idx
  ON work_item (tenant_id, due_at)
  WHERE due_at IS NOT NULL
    AND deleted_at IS NULL AND archived_at IS NULL AND is_completed = false
    AND (due_soon_announced_at IS NULL OR overdue_announced_at IS NULL);
