-- The key behind 0144's column: a notification names a grant of its own workspace, and goes with
-- it. A role taken away before its mail was sent takes the record along, so nobody is told of a role
-- they no longer hold. NOT VALID and then VALIDATE, so the check of the existing rows - all of which
-- have no grant - takes no lock that stops writes.
--
-- The one-subject check is widened by the column, as 0138 widened it: a message is about one thing.
-- Every stored row satisfies the wider check.
--
-- Expand only (rule 12): a pod of the previous release writes no `membership_id`, so neither
-- constraint can refuse anything it does.

-- +goose Up
ALTER TABLE notification ADD CONSTRAINT notification_membership_id_fkey
  FOREIGN KEY (tenant_id, membership_id) REFERENCES membership (tenant_id, id)
  ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_membership_id_fkey;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id) <= 1);

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
