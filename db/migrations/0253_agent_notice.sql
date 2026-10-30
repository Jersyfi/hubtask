-- An agent's owner is told when it did what its policy lets it do with them told (UC-AGT-02,
-- NOTIFY): a notification whose subject is the agent. The mail reads the agent's name at delivery,
-- so the record holds an identifier and no content (rule 10).
--
-- Expand only (rule 12): a nullable column nothing of the previous release writes, and constraints
-- every stored row satisfies, added NOT VALID and then validated.
-- +goose Up
ALTER TABLE notification ADD COLUMN IF NOT EXISTS agent_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_agent_id_fkey
  FOREIGN KEY (tenant_id, agent_id) REFERENCES agent (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_agent_id_fkey;
CREATE INDEX IF NOT EXISTS notification_agent_idx
  ON notification (tenant_id, agent_id) WHERE agent_id IS NOT NULL;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id) <= 1)
  NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_one_subject_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
