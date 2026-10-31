-- A person's own automation rules (configuration/model.md §1, UC-AUT-01 checks 10 and 11):
-- scope_type PERSON names the owner's account in scope_id, and the rule acts as them alone.
--
-- Expand only (rule 12): a widened CHECK accepts every row the old one did, so an old pod writing
-- the three levels it knows is unaffected. The new CHECK holds no existing row (none is PERSON);
-- it is added NOT VALID and validated after, so the table is not held while it is read.
-- +goose Up
ALTER TABLE automation_rule DROP CONSTRAINT IF EXISTS automation_rule_scope_type_check;
ALTER TABLE automation_rule ADD CONSTRAINT automation_rule_scope_type_check
  CHECK (scope_type IN ('TENANT','HUB','COLLECTION','PERSON'));
ALTER TABLE automation_rule ADD CONSTRAINT automation_rule_person_run_as_check
  CHECK (scope_type <> 'PERSON' OR (scope_id IS NOT NULL AND run_as = scope_id)) NOT VALID;
ALTER TABLE automation_rule VALIDATE CONSTRAINT automation_rule_person_run_as_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
