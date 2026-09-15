-- The search remembers which configuration built each document (M-09, ADR-0034).
--
-- ADR-0034 built the language-dependent document in 0.3.0 and left one sentence for later: a
-- mapping row comes with a migration to rebuild the documents that carry the tag. It says
-- nothing about the other way a configuration arrives - an installation upgrades its PostgreSQL,
-- or an operator installs one - after which every entry that was indexed as `simple` for lack of
-- it stays `simple` until it happens to be edited: a search that answers a shorter list than the
-- truth, permanently, for the entries written first.
--
-- So a row now knows what built it. `search_configuration` is the name of the text search
-- configuration the trigger resolved when it wrote the document, filled beside the document by the
-- same trigger; a row whose stored name differs from what hubtask_text_config() answers *today* is
-- stale, and the reindex use case rewrites exactly those, in batches, as a job. NULL means "before
-- this migration", which is treated as stale - not backfilled here, because a backfill would be
-- the rewrite this exists to make selective, and the operation does it where an administrator
-- asks.
--
-- Expand only: a nullable column with no default is a catalogue change, the trigger already fires
-- on every write that changes the document, and the previous version's code neither reads nor
-- writes the column.

-- +goose Up

ALTER TABLE work_item ADD COLUMN IF NOT EXISTS search_configuration text;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.notes);
  NEW.search_configuration := hubtask_text_config(NEW.content_language)::text;
  RETURN NEW;
END $$;
-- +goose StatementEnd
