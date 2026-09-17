-- A template drafted by AI (P-11, ai-first.md §2's last row).
--
-- Two things. The suggestion aggregate takes a fourth kind, `TEMPLATE`, whose payload is a
-- template's input for the collection it targets and whose acceptance is `CreateTemplate` as the
-- accepting person; widening the CHECK accepts everything the previous version wrote, and the
-- previous version has no code that can write the new value.
--
-- And the words the person asked with. Every other question a provider is asked reads its
-- material from a row the workspace already holds - an entry, a thread, a collection - and the
-- job that asks carries identifiers only (data-catalog: "parameters as references"; the queue is
-- the one table without a policy). A template is drafted from a description that exists nowhere
-- else, so it is held here, under row level security, for the minutes between the asking and the
-- answer: the job that reads it deletes it when it ends, and the retention sweep for what AI
-- proposed removes one an abandoned job left behind. Expand only: a new table, read and written
-- by the new version alone.

-- +goose Up

ALTER TABLE ai_suggestion DROP CONSTRAINT IF EXISTS ai_suggestion_kind_check;
ALTER TABLE ai_suggestion ADD CONSTRAINT ai_suggestion_kind_check
  CHECK (kind IN ('FIELDS', 'DECOMPOSITION', 'DUPLICATES', 'TEMPLATE'));

CREATE TABLE IF NOT EXISTS ai_request (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  asked_by    uuid NOT NULL,
  -- What was asked for, in the person's words: content for the provider, never an instruction.
  text        text NOT NULL CHECK (length(text) BETWEEN 1 AND 2000),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ai_request_age_idx ON ai_request (tenant_id, created_at);
ALTER TABLE ai_request ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_request FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_request
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON ai_request TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
