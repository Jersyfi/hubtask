-- A rule drafted from a sentence (UC-AUT-10): kind RULE, about the ACCOUNT that asked - a rule
-- does not exist yet, and only the person who asked may read the draft.
--
-- Expand only: both constraints are widened, no row changes, and a process of the previous release
-- never writes or reads the new values, so the rolling update needs nothing else (rule 12).

-- +goose Up
ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_target_type_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_target_type_check
  CHECK (target_type IN ('WORK_ITEM', 'JUMBLE_ENTRY', 'CONTAINER', 'ACCOUNT'));

ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_kind_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_kind_check
  CHECK (kind IN ('FIELDS', 'DECOMPOSITION', 'DUPLICATES', 'TEMPLATE', 'RULE'));

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
