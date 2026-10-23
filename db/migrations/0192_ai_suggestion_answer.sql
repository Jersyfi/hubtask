-- An answer to a person's question about the workspace (UC-AI-08): kind ANSWER, about the ACCOUNT
-- that asked - only they read it, and it performs no action.
--
-- Expand only: the constraint is widened, no row changes, and a process of the previous release
-- never writes or reads the new value, so the rolling update needs nothing else (rule 12).

-- +goose Up
ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_kind_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_kind_check
  CHECK (kind IN ('FIELDS', 'DECOMPOSITION', 'DUPLICATES', 'TEMPLATE', 'RULE', 'ANSWER'));

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
