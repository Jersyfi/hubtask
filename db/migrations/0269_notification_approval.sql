-- The people an approval counts are told it waits for their yes, reminded while it waits, and the
-- person who asked is told when a level is overdue (UC-SVC-08 check 5, UC-SVC-09 check 4): a
-- notification whose subject is the approval. The mail reads the entry's title at delivery, so the
-- record holds an identifier and no content (rule 10), and an approval gone with its entry takes
-- its records along. The APPROVAL timer purpose was admitted by migration 0232 already.
--
-- Expand only (rule 12): a nullable column nothing of the previous release writes, and constraints
-- every stored row satisfies, added NOT VALID and then validated.
-- +goose Up
ALTER TABLE notification ADD COLUMN IF NOT EXISTS approval_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_approval_id_fkey
  FOREIGN KEY (tenant_id, approval_id) REFERENCES approval (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_approval_id_fkey;
-- The cascade a deleted approval sets off reads this side.
CREATE INDEX IF NOT EXISTS notification_approval_idx
  ON notification (tenant_id, approval_id) WHERE approval_id IS NOT NULL;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id) <= 1)
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
