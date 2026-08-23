-- A deleted comment keeps its row and loses its text (C-03, domain-model.md §3.5).
--
-- The tombstone keeps the thread readable - identity, author and timestamps stay, so a reply does
-- not dangle - and the text is cleared rather than hidden, because a deletion that only hid the
-- words would be retained personal content with no purpose (data-protection.md; the comment row of
-- the data catalogue is PERSONAL_CONTENT). The original CHECK insisted on a non-empty body for
-- every row, which would refuse exactly that clearing, so the rule becomes conditional: a living
-- comment carries 1 to 20000 characters, a tombstone carries none.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the new constraint is
-- added NOT VALID - no scan under the ACCESS EXCLUSIVE lock, only a catalogue change - and
-- validated in a second statement, which takes a lock writes can live with. Every row that
-- satisfied the old rule satisfies the new one, so the validation cannot fail.

-- +goose Up

ALTER TABLE comment DROP CONSTRAINT comment_body_check;
ALTER TABLE comment ADD CONSTRAINT comment_body_check
  CHECK (deleted_at IS NOT NULL OR length(body) BETWEEN 1 AND 20000) NOT VALID;
ALTER TABLE comment VALIDATE CONSTRAINT comment_body_check;
