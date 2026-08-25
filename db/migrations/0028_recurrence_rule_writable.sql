-- The series becomes a row somebody edits (D-04).
--
-- The table has existed since 0001_init with no writer, which is why it carries no `updated_at`:
-- the stamp an edit leaves arrives here, with the first writer, rather than being invented by
-- whoever needed it later. `version` was already there.
--
-- The unique index is the invariant the two pointers rest on. The entry points at its rule
-- (work_item.recurrence_rule_id) and the rule points back at its entry (source_item_id); without
-- "one rule per entry" the two could disagree, and which of two rules an entry repeated by would
-- be whichever pointer was read. It is also the index the read runs on: every use case here starts
-- from the entry.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the column is nullable
-- with no default, so the table is not rewritten, and no row exists to conflict with the index -
-- nothing has ever written this table. Both are built CONCURRENTLY where that is possible, which
-- is why this file runs outside a transaction; an interrupted CONCURRENTLY build leaves an invalid
-- index that IF NOT EXISTS would skip - drop it and run again.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE recurrence_rule ADD COLUMN IF NOT EXISTS updated_at timestamptz;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS recurrence_rule_source_idx
  ON recurrence_rule (source_item_id);
