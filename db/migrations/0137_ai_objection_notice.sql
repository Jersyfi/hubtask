-- A person told once that their objection to AI stopped taking effect (data-protection.md §4.1).
--
-- When a workspace makes AI part of everybody's work, every person whose objection stood is told
-- so once, in the web app: a notice they acknowledge. The acknowledgement is this row, and the
-- notice is pending while it is older than the moment the workspace took that position - so a
-- second switch tells them again, and nobody is told twice about one. In the web app rather than
-- by mail, because an account without an address is told as surely as one with.
--
-- Expand only: a new table nothing older reads. Not in the archive: after a restore a person is
-- told again, which harms nobody.

-- +goose Up
CREATE TABLE IF NOT EXISTS ai_objection_notice (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  noticed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT ai_objection_notice_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE ai_objection_notice ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_objection_notice FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'ai_objection_notice' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON ai_objection_notice
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON ai_objection_notice TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
