-- Attachments join the OR-set tags (C-06).
--
-- An entry's attachments are a set, and offline-sync.md §4.2 says what that means: additions and
-- removals carry their own tags, so that a file attached on one device is not lost when another
-- device detached a different one. `set_element` is already the table that holds those tags for
-- labels, members and watchers; the only thing in the way was the CHECK naming the three sets by
-- hand.
--
-- Widened rather than replaced, and in that order: the new constraint is weaker than the old one,
-- so every existing row satisfies it and the pair can coexist for the length of a rolling update -
-- during which old code writes only the three old names and is refused nothing. The narrow one
-- goes afterwards (CLAUDE.md rule 12, ADR-0003). NOT VALID keeps the catalogue lock brief; the
-- validation is a second statement writes can live with.

-- +goose Up

ALTER TABLE set_element ADD CONSTRAINT set_element_set_name_known CHECK (
  set_name IN ('labels', 'members', 'watchers', 'attachments')
) NOT VALID;
ALTER TABLE set_element VALIDATE CONSTRAINT set_element_set_name_known;

ALTER TABLE set_element DROP CONSTRAINT set_element_set_name_check;
