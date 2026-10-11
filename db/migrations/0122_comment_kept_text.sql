-- The comments whose text a legal hold keeps after their deletion (data-retention.md §4).
--
-- A deleted comment keeps its text while a hold covers it, and the retention pass clears the text
-- once none does. The pass reads exactly these rows on every run, and they are few: only deletions
-- under a hold. Partial on the same predicate, so the index is the size of what a hold keeps rather
-- than of every comment of the installation, and a write to a living comment never touches it.
-- `tenant_id` leads for row level security's predicate, `id` for the pass's keyset (0008's
-- reasoning).
--
-- Expand only: a pod of the previous release clears the text on every deletion and never matches
-- the predicate (rule 12). CONCURRENTLY, so no write to `comment` waits while it builds - which is
-- why this migration takes no transaction; IF NOT EXISTS covers the retry after an interrupted
-- build, which leaves an invalid index behind under the same name.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS comment_kept_text_idx
  ON comment (tenant_id, id)
  WHERE deleted_at IS NOT NULL AND body <> '';

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS comment_kept_text_idx;
