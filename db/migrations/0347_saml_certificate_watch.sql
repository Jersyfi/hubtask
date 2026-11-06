-- A SAML provider's certificates are watched so that the workspace's administrators are told 30, 7
-- and 1 days before the last one ends (identity.md §26.1): an ended certificate refuses every
-- answer and nobody signs in through the provider any more.
--
-- `saml_expiry_told_for` is the end a warning was given for and `saml_expiry_told_days` the
-- smallest warning given for it, so a new certificate (a new end) is warned of again without a
-- reset on save. `notification.identity_provider_id` is the message's subject, the twelfth the
-- one-subject check counts.
--
-- The watch is a per-workspace job seeded by the write that configures a SAML provider; the
-- workspaces whose providers were saved before this are seeded here once. An older binary never
-- writes the columns and leaves the job waiting for a pod that knows its kind.
--
-- Safe for a rolling update: nullable columns, a NOT VALID foreign key and check, validated with
-- the index in 0348 outside a transaction.

-- +goose Up

ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS saml_expiry_told_for timestamptz;
ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS saml_expiry_told_days smallint;

ALTER TABLE notification ADD COLUMN IF NOT EXISTS identity_provider_id uuid;
ALTER TABLE notification ADD CONSTRAINT notification_identity_provider_id_fkey
  FOREIGN KEY (identity_provider_id) REFERENCES identity_provider (id) ON DELETE CASCADE NOT VALID;

ALTER TABLE notification DROP CONSTRAINT notification_one_subject_check;
ALTER TABLE notification ADD CONSTRAINT notification_one_subject_check
  CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id, agent_id,
    whiteboard_id, approval_id, baseline_id, mailbox_id, identity_provider_id) <= 1)
  NOT VALID;

INSERT INTO job (id, tenant_id, kind, dedupe_key, run_at)
SELECT gen_random_uuid(), watched.tenant_id, 'identity.saml_certificate_watch',
  'saml-certificates:' || watched.tenant_id::text, now()
FROM (SELECT DISTINCT tenant_id FROM identity_provider WHERE kind = 'SAML' AND tenant_id IS NOT NULL) AS watched
ON CONFLICT (kind, dedupe_key) WHERE dedupe_key IS NOT NULL AND state IN ('PENDING', 'RUNNING') DO NOTHING;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
