-- A suggestion names the registered agent that asked for it, so the mark a person's acceptance
-- leaves names it too (UC-AGT-05 check 1, ai-first.md AI-2). Empty where a person or a rule asked.
--
-- Expand only (rule 12): a nullable column nothing of the previous release writes or reads, and a
-- constraint every stored row satisfies, added NOT VALID and then validated.
-- +goose Up
ALTER TABLE ai_suggestion ADD COLUMN IF NOT EXISTS agent_id uuid;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_agent_id_fkey
  FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE ai_suggestion VALIDATE CONSTRAINT ai_suggestion_agent_id_fkey;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
