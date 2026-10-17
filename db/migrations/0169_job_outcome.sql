-- How a job that succeeded ended, where its kind says more than that it finished: a question to the
-- workspace's AI answers whether it proposed something, found nothing to change, had nothing to work
-- with, or met a person who keeps their content out since asking. Without it a job that proposed
-- nothing reads exactly like one still on its way, and the person who asked waits for nothing.
--
-- A message code only, never a message and never anything the job was working on (rule 10): the
-- `job` table is the one table without row level security (0001_init.sql), and only the runner's
-- completion writes this column, from the handler's own code. NULL for every other job.
--
-- Expand only (rule 12): a nullable column with no default, which a pod of the previous release
-- neither reads nor writes.

-- +goose Up
ALTER TABLE job ADD COLUMN IF NOT EXISTS outcome text;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
