-- An approved held act is marked performing until its answer is recorded, so a process that dies
-- between the decision and the act leaves a mark a later check can heal: the request is then
-- recorded FAILED with agents.request_interrupted instead of staying APPROVED unperformed.
--
-- Expand only (rule 12): a new column with a default; a process of the previous release never
-- sets it, so nothing it approves is ever healed by mistake.
-- +goose Up
ALTER TABLE agent_request ADD COLUMN IF NOT EXISTS performing boolean NOT NULL DEFAULT false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
