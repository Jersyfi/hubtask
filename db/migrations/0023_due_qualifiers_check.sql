-- The due qualifiers cannot outlive their date (D-01).
--
-- i18n-l10n.md §4 and the backlog both say it in one sentence: a due_time_zone or a due_date_only
-- without a due_at cannot be stored. The application refuses the combination at construction, and
-- this constraint is what makes the sentence true of the table rather than of one writer - the
-- offline merge, a future bulk path and every statement yet to be written meet the same wall, the
-- way the cover's CHECK (migration 0013) guards its three columns.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the schedule columns
-- have carried NULL on every row since 0001_init - this milestone brings their first writer - so
-- the validation scan finds nothing to object to, and old code, which never writes the columns,
-- cannot violate it.

-- +goose Up

ALTER TABLE work_item
  ADD CONSTRAINT wi_due_qualifiers_check
  CHECK (due_at IS NOT NULL OR (due_time_zone IS NULL AND NOT due_date_only));
