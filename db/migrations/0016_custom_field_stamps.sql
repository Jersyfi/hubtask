-- The custom field definition gains what every other editable row has (C-07).
--
-- `custom_field_definition` has existed since 0001 and nothing has ever written it, so it never
-- needed the three columns an edit does: when it was made, when it last moved, and the optimistic
-- lock an `If-Match` is compared against. A definition is edited - its options are narrowed, a
-- type is added to `applies_to` - and two people editing the same one has to be a conflict rather
-- than a silent overwrite (api-guidelines.md §5).
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the columns arrive
-- with constant defaults, which PostgreSQL records in the catalogue rather than writing into every
-- row, and there are no rows to write anyway. Old code selects the columns it knows and is
-- unaffected.

-- +goose Up

ALTER TABLE custom_field_definition
  ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN version    integer     NOT NULL DEFAULT 1;
