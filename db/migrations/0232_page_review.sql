-- A page's owner is reminded when its verification runs out (UC-DOC-12 check 4): a durable timer
-- of its own purpose counts down to the date, and the reminder is a notification whose subject is
-- the page. The mail reads the page's title at delivery, so the record holds an identifier and no
-- content (rule 10), and a page purged before then takes its record along.
--
-- Expand only (rule 12): a purpose and a nullable column nothing of the previous release writes,
-- and constraints every stored row satisfies. NOT VALID and then VALIDATE, so checking the rows
-- takes no lock that stops writes.
-- +goose Up
ALTER TABLE durable_timer DROP CONSTRAINT IF EXISTS durable_timer_purpose_check;
ALTER TABLE durable_timer ADD CONSTRAINT durable_timer_purpose_check
  CHECK (purpose IN ('SLA', 'APPROVAL', 'EFFECTIVENESS_CHECK', 'PAGE_REVIEW')) NOT VALID;
ALTER TABLE durable_timer VALIDATE CONSTRAINT durable_timer_purpose_check;

ALTER TABLE notification ADD COLUMN IF NOT EXISTS page_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_page_id_fkey
  FOREIGN KEY (tenant_id, page_id) REFERENCES page (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE notification VALIDATE CONSTRAINT notification_page_id_fkey;
-- The cascade a purged page sets off reads this side.
CREATE INDEX IF NOT EXISTS notification_page_idx
  ON notification (tenant_id, page_id) WHERE page_id IS NOT NULL;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id) <= 1)
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
