-- The reminder becomes a row somebody edits (D-02).
--
-- The table has existed since 0001_init with no writer at all, which is why it carries neither of
-- the two columns every edited row in this schema has: `version` for the optimistic lock every
-- update takes (api-guidelines.md §5) and `updated_at` for the stamp an edit leaves. Both arrive
-- here, with the reminder's first writer, rather than being invented by whoever needed them later.
--
-- The list index is the other half. reminder_due_idx (tenant_id, state, fire_at) serves the
-- scheduler's question - what is due next - and answers nothing about one entry's reminders, which
-- is the question the API asks; reminder_item_idx is that question's index, and its columns are
-- the ORDER BY the list runs on.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): both columns are added
-- with a default that needs no table rewrite on PostgreSQL 16 and 17, no row exists to backfill -
-- nothing has ever written this table - and old code, which does not know the columns, keeps
-- working because the default supplies them. The index is built CONCURRENTLY, so no write waits
-- for it, which is why this file runs outside a transaction. The same operational caveat as
-- migration 0004: an interrupted CONCURRENTLY build leaves an invalid index that IF NOT EXISTS
-- would skip - drop it and run again.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE reminder ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;
ALTER TABLE reminder ADD COLUMN IF NOT EXISTS updated_at timestamptz;

CREATE INDEX CONCURRENTLY IF NOT EXISTS reminder_item_idx
  ON reminder (item_id, created_at, id);
