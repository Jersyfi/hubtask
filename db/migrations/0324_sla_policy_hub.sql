-- An SLA policy defined at a hub (configuration model §1, §2; UC-SVC-02 check 13): a hub's own
-- policy measures only the entries inside that hub and is tried there before the workspace's, and
-- a copy of a shared one records the key it was made from. Keys stay unique in the workspace, as
-- 0317 chose for entry types, so an entry's goal record (`entry_sla.policy_key`) names one policy.
--
-- The hub goes with its policies: a hub is purged after its collections and entries, and the
-- goal records of those entries went with them. `copied_from` is a key, not a reference: the copy
-- keeps saying where it came from after the source is gone.
--
-- Expand only (rule 12): two nullable columns, the reference added NOT VALID and validated after,
-- so the table is not held while it is read. Null is the whole workspace, which is what every
-- policy written before this migration, and every one an older binary writes, means.

-- +goose Up
ALTER TABLE sla_policy ADD COLUMN IF NOT EXISTS hub_id uuid;
ALTER TABLE sla_policy ADD COLUMN IF NOT EXISTS copied_from text
  CHECK (copied_from IS NULL OR copied_from ~ '^[a-z][a-z0-9_]{0,49}$');
ALTER TABLE sla_policy
  ADD CONSTRAINT sla_policy_hub_fkey
  FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE NOT VALID;
ALTER TABLE sla_policy VALIDATE CONSTRAINT sla_policy_hub_fkey;
CREATE INDEX IF NOT EXISTS sla_policy_hub_idx ON sla_policy (tenant_id, hub_id) WHERE hub_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
