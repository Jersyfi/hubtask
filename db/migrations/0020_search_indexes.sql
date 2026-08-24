-- What the search reads through (C-08, ADR-0034).
--
-- Two indexes for two kinds of writing. The GIN over the document answers `@@` for every language
-- whose words are separated by spaces. The trigram index answers the ones whose words are not:
-- CJK and Thai produce one token per run of characters, and a tsquery cannot find a substring of a
-- token - so for those scripts the trigram index is not an optimisation of the search, it is the
-- search (i18n-l10n.md §5).
--
-- The trigram index is an expression over title and notes rather than a second stored copy of
-- them. The expression is written the same way in the compiled statement, because the planner
-- matches an expression index by its tree: a coalesce spelled differently there would build this
-- index and never use it (infrastructure/postgres/query/Compiler.go, searchText).
--
-- Forward-only and safe for a rolling update: CONCURRENTLY, outside a transaction, IF NOT EXISTS
-- for CONCURRENTLY's interrupted-build failure mode (drop the invalid index and run again).

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_search_document_idx
  ON work_item USING gin (search_document);

CREATE INDEX CONCURRENTLY IF NOT EXISTS wi_search_trgm_idx
  ON work_item USING gin ((coalesce(title, '') || ' ' || coalesce(notes, '')) gin_trgm_ops);
