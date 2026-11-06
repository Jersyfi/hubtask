-- Validates 0376's keys, after repairing what deletions before them left behind.
--
-- A connector, hook or agent naming an account that no longer exists is moved to the workspace's
-- oldest owner grant held by a person who can act (WorkspaceOwnerPaths' rule), or else to the
-- workspace's oldest account, and a connector or hook is switched off: it acted as nobody and
-- failed on every run, and an erasure never switches one on. Nothing is deleted. Then the keys are
-- validated without a transaction, under a lock writes pass.

-- +goose NO TRANSACTION

-- +goose Up
-- +goose StatementBegin
DO $repair$
BEGIN
  CREATE TEMPORARY TABLE heir ON COMMIT DROP AS
  SELECT t.id AS tenant_id,
         coalesce(
           (SELECT a.id FROM membership m JOIN account a ON a.id = m.account_id
            WHERE m.tenant_id = t.id AND m.scope_type = 'TENANT' AND m.role = 'OWNER'
              AND a.kind = 'USER' AND a.status IN ('ACTIVE', 'RESTRICTED') AND a.deleted_at IS NULL
            ORDER BY m.id LIMIT 1),
           (SELECT a.id FROM account a WHERE a.tenant_id = t.id ORDER BY a.id LIMIT 1)) AS account_id
  FROM tenant t;

  UPDATE cmdb_connector c SET run_as = h.account_id, enabled = false, updated_at = now(), version = c.version + 1
  FROM heir h
  WHERE h.tenant_id = c.tenant_id AND h.account_id IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM account a WHERE a.tenant_id = c.tenant_id AND a.id = c.run_as);
  UPDATE code_hook k SET run_as = h.account_id, enabled = false, updated_at = now()
  FROM heir h
  WHERE h.tenant_id = k.tenant_id AND h.account_id IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM account a WHERE a.tenant_id = k.tenant_id AND a.id = k.run_as);
  UPDATE agent g SET owner_id = h.account_id, updated_at = now(), version = g.version + 1
  FROM heir h
  WHERE h.tenant_id = g.tenant_id AND h.account_id IS NOT NULL
    AND NOT EXISTS (SELECT 1 FROM account a WHERE a.tenant_id = g.tenant_id AND a.id = g.owner_id);
END $repair$;
-- +goose StatementEnd

ALTER TABLE cmdb_connector VALIDATE CONSTRAINT cmdb_connector_run_as_fkey;
ALTER TABLE code_hook VALIDATE CONSTRAINT code_hook_run_as_fkey;
ALTER TABLE agent VALIDATE CONSTRAINT agent_owner_fkey;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
