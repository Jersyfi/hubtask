-- What a search reads of an entry's attachments (F-9): the name each file arrived under, so that
-- an entry is found by the file it carries.
--
-- An expression index under `simple`, for migration 0225's reason. The dot, the underscore and the
-- hyphen become spaces first, because the parser reads "budget_2026.pdf" as one file token and a
-- search for "budget" would never meet it; translate is IMMUTABLE. The compiled statement writes
-- the same expression (infrastructure/postgres/query/Compiler.go, attachedMatch).
--
-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for an interrupted build (drop the invalid index and run again). Nothing older reads it.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS media_object_name_search_idx
  ON media_object USING gin (to_tsvector('simple'::regconfig, translate(file_name, '._-', '   ')))
  WHERE file_name IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
