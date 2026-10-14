-- The parameters of the refusal a job met, beside its code: a suggestion's job that finds the
-- workspace's AI budget spent ends with `ai.unavailable`, and the sentence a person reads needs its
-- `reason` and `renews_at` - without them it reads "AI is switched off", which is not what happened.
--
-- Codes and moments only, never a message or anything the job was working on (rule 10): only the
-- runner's refusal path writes it, from the refusal's own parameters. NULL for every other job.
--
-- Expand only (rule 12): a nullable column with no default, which a pod of the previous release
-- neither reads nor writes.

-- +goose Up
ALTER TABLE job ADD COLUMN IF NOT EXISTS last_error_params jsonb;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
