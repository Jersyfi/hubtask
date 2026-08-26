-- The template becomes a row somebody writes (D-06).
--
-- The table has existed since 0001_init with no writer, which is why it carries no `updated_at`
-- and no index beyond its primary key. Both arrive here, with the first writer.
--
-- The unique index is the one rule a name has: two live templates in one scope may not share it.
-- Partial on the live rows, which is what gives the soft delete its shape - a deleted template
-- frees its name, and a template defined afterwards under that name is a new one rather than the
-- old one coming back (the C-07 lesson, and the same index shape cfd_key_uq has). The coalesce is
-- not decoration: scope_id is NULL for a workspace-wide template, NULLs are distinct to a unique
-- index, and without it the one scope that has no container would be the one scope with no rule.
--
-- template_tenant_id_uq is the tenant-first unique index every table a tenant-scoped foreign key
-- could point at carries (ADR-0024, migration 0004). Nothing references a template yet - an
-- instantiated tree is ordinary entries that outlive it - and the index is here so that the first
-- thing that wants to does not have to add it under load.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the column is nullable
-- with no default, no row exists to conflict with the indexes - nothing has ever written this
-- table - and everything is built CONCURRENTLY, which is why this file runs outside a transaction.
-- An interrupted CONCURRENTLY build leaves an invalid index that IF NOT EXISTS would skip - drop
-- it and run again.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE template ADD COLUMN IF NOT EXISTS updated_at timestamptz;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS template_tenant_id_uq
  ON template (tenant_id, id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS template_name_uq
  ON template (
    tenant_id, scope_type,
    coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid),
    name
  )
  WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS template_scope_idx
  ON template (tenant_id, scope_type, scope_id, created_at DESC)
  WHERE deleted_at IS NULL;
