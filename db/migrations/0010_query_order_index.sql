-- The index the query language reads through in its default order (B-12).
--
-- `POST /items:query` is anchored to a scope and returns its page in a sort order the caller
-- chooses; the order it chooses by default, and the one every board and list is drawn in, is the
-- manual one. That query is `collection_id` equality with `(order_key, id)` order, and no existing
-- index serves it.
--
-- `wi_level_order_idx` is `(tenant_id, collection_id, parent_id, order_key, id)`: it has the right
-- leading column and then `parent_id` in the middle, which a query spanning a whole collection does
-- not constrain - so the ordering falls back to a sort of everything the collection holds.
-- `wi_board_idx` has `bucket_id` in that position and orders without the collation this query
-- compares in, and is partial on `archived_at IS NULL`, which `include_archived: true` needs the
-- other half of.
--
-- `order_key COLLATE "C"` is explicit here and in the compiled statement, for the reason migration
-- 0007 states: a rank key is a fractional index whose scheme rests on byte order, and a glibc
-- `en_US.UTF-8` would make the index and the keyset comparison disagree with the domain about what
-- "next" means. `id` is last because the cursor is a keyset over the pair.
--
-- Partial on `deleted_at IS NULL` only: the trash is its own view and is never in a query's answer
-- unless `include_trashed` asks for it, in which case a sequential scan of one collection is the
-- honest price of asking. Archived entries stay in the index because the query serves both
-- positions of `include_archived`.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): CONCURRENTLY, so no
-- write is blocked while it builds, which is why this migration takes no transaction. IF NOT EXISTS
-- covers the failure mode CONCURRENTLY has - an interrupted build leaves an invalid index behind,
-- and the retry must not fail on the name.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_query_order_idx
  ON work_item (tenant_id, collection_id, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS wi_query_order_idx;
