-- What a search reads beyond the entry's own document (F-9): its comments and its text-valued
-- custom fields, so that an entry is found by what was said about it and by what it carries.
--
-- Expression indexes under `simple`, because an index expression must be IMMUTABLE and the
-- per-language resolver is STABLE (migration 0019). The compiled statement writes the same
-- expression, since the planner matches an expression index by its tree
-- (infrastructure/postgres/query/Compiler.go, attachedMatch).
--
-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for an interrupted build (drop the invalid index and run again). Nothing older reads them.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS comment_search_idx
  ON comment USING gin (to_tsvector('simple'::regconfig, body)) WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS efv_search_idx
  ON entry_field_value USING gin (to_tsvector('simple'::regconfig, text)) WHERE text IS NOT NULL;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
