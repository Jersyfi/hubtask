-- Which definition each custom field value was written under (C-07).
--
-- The acceptance is exact: a definition deleted and recreated under the same key must not
-- resurrect what the old one held, while the values themselves stay in the rows. Visibility by key
-- alone cannot satisfy both - the moment the key has a live definition again, the old value would
-- be back - so each value remembers the *identity* of the definition it was written under, and a
-- read shows a value only when exactly that definition still lives. A recreated key is a new
-- definition with a new id, standing behind nothing it did not write.
--
-- A jsonb map beside the values rather than a join table, because the two move together: one
-- statement writes a key's value and its ref under one version predicate, and a table would make
-- that two statements whose halves a crash could separate.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): a constant default,
-- recorded in the catalogue rather than written into every row. Old code neither selects nor
-- writes the column and is unaffected; values written before this migration have no ref and are
-- therefore hidden - and there are none, because nothing wrote custom fields before C-07.

-- +goose Up

ALTER TABLE work_item
  ADD COLUMN custom_field_refs jsonb NOT NULL DEFAULT '{}'::jsonb;
