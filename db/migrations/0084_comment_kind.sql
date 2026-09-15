-- A comment the server filed (N-06, offline-sync.md §5).
--
-- When two versions of a free-text field collide, one loses - and it does not disappear: the
-- displaced version is filed as a system comment on the entry, recoverable, and the push answers
-- with both values. A system comment is told apart from what somebody wrote by its kind, and its
-- heading is a message code with parameters rather than a sentence the server composed (rule 8):
-- "Diverging version from Anna, 14 Aug 09:12" is what a client renders from `sync.displaced_version`,
-- the author and the reading. The body is the text that lost, which is the person's own words.
--
-- Expand only: a column with a default and two nullable ones; the previous version reads every
-- comment as it did and writes rows the default makes USER.

-- +goose Up

ALTER TABLE comment
  ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'USER'
    CONSTRAINT comment_kind_known CHECK (kind IN ('USER', 'SYSTEM')),
  ADD COLUMN IF NOT EXISTS system_code text,
  ADD COLUMN IF NOT EXISTS system_params jsonb;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
