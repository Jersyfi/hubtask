-- A plan whose forecast end slips past its latest baseline's by more than its time tolerance is
-- in exception, and the person who decided the tolerance - the level above, as PRINCE2 has it - is
-- told (standards.md P12): a notification whose subject is the baseline the plan slipped from. The
-- mail reads the plan's place at delivery, so the record holds an identifier and no content (rule
-- 10); a baseline gone with its plan takes its records along. One record per baseline and
-- recipient: the exception is told once, and a new baseline - the plan agreed again - may raise
-- the next.
--
-- Expand only (rule 12): a nullable column nothing of the previous release writes, constraints
-- every stored row satisfies, added NOT VALID and then validated, and an index no stored row
-- conflicts with.
-- +goose Up
ALTER TABLE notification ADD COLUMN IF NOT EXISTS baseline_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_baseline_id_fkey
  FOREIGN KEY (tenant_id, baseline_id) REFERENCES plan_baseline (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_baseline_id_fkey;
CREATE UNIQUE INDEX IF NOT EXISTS notification_baseline_once
  ON notification (tenant_id, baseline_id, recipient_id, channel) WHERE baseline_id IS NOT NULL;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id, baseline_id) <= 1)
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
