-- The notes a push keeps are bounded as the notes are, not as a comment (offline-sync.md §5).
--
-- A system comment holds the notes of an entry that lost a push. The notes carry no length bound
-- of their own (an unbounded text column; only the request body limit caps them), so notes longer
-- than a comment's 20000 characters could be written but not kept, and the version that loses
-- would be lost. The bound of a system comment is the domain's (Comment.go, NewComment); the
-- column only insists that a living one is not empty. A comment somebody writes keeps its 1 to
-- 20000 characters.
--
-- Forward-only and safe for a rolling update (rule 12, ADR-0003): the rule only widens, so every
-- row the previous version writes satisfies it. Added NOT VALID; 0447 validates it outside this
-- transaction, and every existing row satisfies the new rule, so the validation cannot fail.

-- +goose Up

ALTER TABLE comment DROP CONSTRAINT comment_body_check;
ALTER TABLE comment ADD CONSTRAINT comment_body_check
  CHECK (
    deleted_at IS NOT NULL
    OR length(body) BETWEEN 1 AND 20000
    OR (kind = 'SYSTEM' AND length(body) >= 1)
  ) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
