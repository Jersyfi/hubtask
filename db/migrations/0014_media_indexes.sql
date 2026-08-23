-- The reads C-06 adds, indexed (ADR-0010: tenant first, because that is what every row level
-- security predicate compares first).
--
-- `item_attachment_media_idx` answers "what references this object" - the reconciliation job's
-- recount and GetMedia's authorisation both walk from the object to its items, and the primary
-- key only walks the other way. `wi_cover_media_idx` is the same question for covers, partial
-- because almost every item has none. `media_object_reconcile_idx` is the orphan sweep's walk:
-- per tenant, by status, reference count and age.
--
-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for CONCURRENTLY's interrupted-build failure mode (drop the invalid index and run again).

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS item_attachment_media_idx
  ON item_attachment (tenant_id, media_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_cover_media_idx
  ON work_item (tenant_id, cover_media_id) WHERE cover_media_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS media_object_reconcile_idx
  ON media_object (tenant_id, status, ref_count, created_at);
