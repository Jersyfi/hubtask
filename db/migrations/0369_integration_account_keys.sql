-- The integrations that act as a person name them by a foreign key (data-protection.md §4).
--
-- `cmdb_connector.run_as`, `code_hook.run_as` and `agent.owner_id` named an account with no key,
-- so an account could go while a switched-off connector, hook or retired agent still named it, and
-- the integration then pointed at nobody. An erasure now moves those to another owner of the
-- workspace first (UC-PRV-01 check 12); these keys refuse any deletion that would still leave one
-- behind. Composite over the tenant (multi-tenancy.md §2.1.2) and NO ACTION, checked at the end
-- of the statement, so a workspace's own deletion, which cascades to both sides, still goes.
--
-- Added NOT VALID: a short lock and no scan, and enforced from now on for every write; 0370
-- repairs what older deletions left and validates them. Expand only and safe for a rolling update:
-- a pod of the previous release never deletes an account an integration names but by an erasure,
-- which refused that for switched-on ones already.

-- +goose Up
ALTER TABLE cmdb_connector ADD CONSTRAINT cmdb_connector_run_as_fkey
  FOREIGN KEY (tenant_id, run_as) REFERENCES account (tenant_id, id) NOT VALID;
ALTER TABLE code_hook ADD CONSTRAINT code_hook_run_as_fkey
  FOREIGN KEY (tenant_id, run_as) REFERENCES account (tenant_id, id) NOT VALID;
ALTER TABLE agent ADD CONSTRAINT agent_owner_fkey
  FOREIGN KEY (tenant_id, owner_id) REFERENCES account (tenant_id, id) NOT VALID;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
