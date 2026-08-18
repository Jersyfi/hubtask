-- Tenant-scoped foreign keys, step 1 of 3: the unique indexes the keys point at (ADR-0024).
--
-- A composite foreign key needs a unique index on exactly the columns it references, so every
-- table that is referenced gains one on (tenant_id, id). The primary key stays on `id` alone:
-- identifiers are UUIDv7 and globally unique, and every query looks a row up by identifier.
--
-- The index is not dead weight beside the primary key. ADR-0010's countermeasures ask for indices
-- that begin with `tenant_id`, because that is what every row level security predicate compares
-- first - this is that index, and the composite key is what finally makes it required.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003):
-- the indexes are built CONCURRENTLY, so no write is blocked while they are built. That is also
-- why this file runs outside a transaction.
--
-- The one operational caveat: a CONCURRENTLY build that is interrupted leaves an invalid index
-- behind, and IF NOT EXISTS would then skip it on a re-run. Step 2 fails loudly in that case - an
-- invalid index cannot back a foreign key - which is the right way round: the migration stops
-- rather than the constraint quietly not being there. Drop the invalid index and run again.

-- +goose NO TRANSACTION

-- +goose Up

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS account_tenant_id_uq
  ON account (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS account_group_tenant_id_uq
  ON account_group (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS automation_rule_tenant_id_uq
  ON automation_rule (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS bucket_tenant_id_uq
  ON bucket (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS comment_tenant_id_uq
  ON comment (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS container_tenant_id_uq
  ON container (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS label_tenant_id_uq
  ON label (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS media_object_tenant_id_uq
  ON media_object (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS saved_view_tenant_id_uq
  ON saved_view (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS webhook_subscription_tenant_id_uq
  ON webhook_subscription (tenant_id, id);
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS work_item_tenant_id_uq
  ON work_item (tenant_id, id);

-- +goose Down

-- The indexes stay. Step 2's constraints depend on them, and dropping an index a constraint needs
-- is not a rollback but a broken schema (ADR-0003: the answer to a bad deploy is a restore).
SELECT 1;
