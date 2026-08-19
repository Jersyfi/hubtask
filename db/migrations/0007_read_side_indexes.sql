-- The index the plain item list reads through (B-04).
--
-- `GET /items` lists one level of one collection in its manual order: the items directly in the
-- collection when no parent is named, that item's children when one is. Both are the same query
-- shape - `(collection_id, parent_id)` equality, `(order_key, id)` order - and neither is served by
-- an index that exists.
--
-- `wi_board_idx` is `(tenant_id, collection_id, bucket_id, order_key)`: it can seek to a collection
-- but then orders by bucket first, so ordering by `order_key` alone means a sort. `wi_parent_idx` is
-- `(tenant_id, parent_id, order_key)`, which serves the children of one item but spans the whole
-- tenant when `parent_id IS NULL` - the tasks, the commonest case of all.
--
-- `id` is the last column because the cursor is a keyset over `(order_key, id)`: the pair is what
-- makes a page boundary unambiguous when two siblings share a rank, and having it in the index means
-- the comparison and the ordering are both index-only (api-guidelines.md §4, "sorting always ends
-- implicitly on id ASC").
--
-- Partial on `deleted_at IS NULL` rather than on archived as well, unlike `wi_board_idx`: the list
-- serves `include_archived` in both positions, and an index that excluded archived rows would be
-- unusable for one of them. Trashed rows are excluded outright - the trash is its own view (B-10).
--
-- `order_key COLLATE "C"` is explicit, and the query says the same. A rank key is a fractional index
-- whose whole scheme rests on byte order - `A`..`Z` head the negative integers and `a`..`z` the
-- non-negative ones, so byte order is what puts "before everything" below "after everything"
-- (core/domain/service/Ordering.go). The database is created `en_US.utf8`, which on the musl image
-- this project ships resolves to byte order anyway; on a glibc managed instance it does not, and the
-- index and the keyset comparison would then disagree with the domain about what "next" means.
-- Stating the collation makes the read path correct on either.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): CONCURRENTLY, so no
-- write is blocked while it builds, which is also why this migration takes no transaction. IF NOT
-- EXISTS covers the one failure mode CONCURRENTLY has - an interrupted build leaves an invalid index
-- behind, and a retry must not then fail on the name.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_level_order_idx
  ON work_item (tenant_id, collection_id, parent_id, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX CONCURRENTLY IF EXISTS wi_level_order_idx;
