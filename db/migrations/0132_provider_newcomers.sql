-- What newcomers get (UC-ID-11 check 5, identity.md §10.4): a workspace's own provider that creates
-- accounts may name one role on the whole workspace, or one group, that somebody it brings in new
-- receives in the same transaction that creates the account. Both NULL is nothing - no access until
-- an administrator gives it - which every existing row keeps.
--
-- The group's key is the tenant-scoped pair, so a row can only name a group of its own workspace,
-- and deleting the group leaves the provider in place and its newcomers with nothing. The CHECK
-- holds what the database can see: one of the two, never on the installation's own rows (a group
-- is a workspace's), never OWNER. Which modes may carry them is the application's rule: a pod of
-- the previous release that changes a row's mode does not know these columns, and a CHECK on the
-- mode would refuse its write mid-rollout.
--
-- Expand only: two nullable columns, a foreign key and a CHECK on columns no row has set yet
-- (rule 12).
-- +goose Up
ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS newcomer_role membership_role;
ALTER TABLE identity_provider ADD COLUMN IF NOT EXISTS newcomer_group_id uuid;

ALTER TABLE identity_provider
  ADD CONSTRAINT identity_provider_newcomer_group_fk
  FOREIGN KEY (tenant_id, newcomer_group_id) REFERENCES account_group (tenant_id, id)
  ON DELETE SET NULL (newcomer_group_id) NOT VALID;
ALTER TABLE identity_provider VALIDATE CONSTRAINT identity_provider_newcomer_group_fk;

ALTER TABLE identity_provider
  ADD CONSTRAINT identity_provider_newcomers_check CHECK (
    (newcomer_role IS NULL OR newcomer_group_id IS NULL)
    AND (tenant_id IS NOT NULL OR (newcomer_role IS NULL AND newcomer_group_id IS NULL))
    AND newcomer_role IS DISTINCT FROM 'OWNER'
  ) NOT VALID;
ALTER TABLE identity_provider VALIDATE CONSTRAINT identity_provider_newcomers_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
