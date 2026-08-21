-- The item history: the deletion path it hangs on, and the index a page is read through (B-11).
--
-- `activity_entry` has been in the schema since 0001 with nothing writing to it. Two things it was
-- missing are what this task needs.
--
-- The first is the foreign key. The data catalogue declares the deletion path for `activity_entry`
-- as CASCADE, "with the item" (docs/privacy/data-catalog.md §3) - and with no key at all, a purge
-- would leave the history of a deleted entry behind, which is a copy of somebody's business content
-- surviving the deletion that was meant to reach it. Tenant-scoped, like every other key in this
-- schema since ADR-0024: referential integrity is checked in triggers that run as the table owner,
-- which row level security does not reach, so a key without the tenant in it lets a cascade in one
-- tenant delete a row in another.
--
-- MATCH SIMPLE is the default and is what keeps `item_id` optional: the column is nullable, an entry
-- without an item is not checked at all, and none is written - the domain refuses one. The column
-- stays nullable rather than being tightened, because `container_id` is there for a container's own
-- history, and this milestone's reader is per item (api-guidelines.md §2).
--
-- The second is the index. A page is read newest first and continues after a boundary, so the walk
-- is a keyset scan on (occurred_at DESC, id DESC) within one item. `activity_item_idx` stops at
-- `occurred_at` and leaves the tie-break to a sort; the new one carries it. It is a strict superset
-- of the old, which is why the old one goes: two indexes with the same leading columns are a write
-- cost on every entry for an answer one of them already gives.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003). The constraint is added
-- NOT VALID, so no existing row is scanned and no long lock is taken, and validated in a second
-- statement that takes only a SHARE UPDATE EXCLUSIVE lock - blocking neither reads nor writes.
-- Both indexes are handled CONCURRENTLY, which is why this migration takes no transaction, and the
-- new one is created before the old one is dropped so that no page is ever read without one.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE activity_entry
  ADD CONSTRAINT activity_entry_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id)
      ON DELETE CASCADE NOT VALID;

ALTER TABLE activity_entry VALIDATE CONSTRAINT activity_entry_item_id_fkey;

CREATE INDEX CONCURRENTLY IF NOT EXISTS activity_page_idx
  ON activity_entry (tenant_id, item_id, occurred_at DESC, id DESC);

DROP INDEX CONCURRENTLY IF EXISTS activity_item_idx;

-- +goose Down

-- The index comes back before the superset goes, for the same reason it went second on the way up.
CREATE INDEX CONCURRENTLY IF NOT EXISTS activity_item_idx
  ON activity_entry (tenant_id, item_id, occurred_at DESC);

DROP INDEX CONCURRENTLY IF EXISTS activity_page_idx;

ALTER TABLE activity_entry DROP CONSTRAINT IF EXISTS activity_entry_item_id_fkey;
