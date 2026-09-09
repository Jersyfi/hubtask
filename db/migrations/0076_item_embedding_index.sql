-- The vector index, built CONCURRENTLY and therefore alone (J-09, ADR-0050).
--
-- Its own migration after the table, the discipline 0019 and 0020 established for the search
-- document: CREATE INDEX CONCURRENTLY cannot run inside a transaction, and a rolling update must
-- not meet an ACCESS EXCLUSIVE lock over a table the previous version is writing to.
--
-- Conditional for the same reason the table is: an installation without pgvector has no table to
-- index, and this migration does nothing there.

-- +goose NO TRANSACTION

-- +goose Up

-- +goose StatementBegin
DO $index$
BEGIN
  IF to_regclass('public.item_embedding') IS NULL THEN
    RETURN;
  END IF;

  -- HNSW rather than IVFFlat: it needs no training pass over data that does not exist yet, which
  -- matters for a table that starts empty and fills as a job embeds. Cosine distance, because the
  -- embedding models in question produce vectors normalised for it.
  --
  -- CONCURRENTLY is what keeps this a rolling-update-safe migration; it cannot be wrapped in the
  -- DO block's implicit transaction, so the statement is executed rather than written inline.
  EXECUTE 'CREATE INDEX CONCURRENTLY IF NOT EXISTS item_embedding_vector_idx '
       || 'ON item_embedding USING hnsw (embedding vector_cosine_ops)';
END $index$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
