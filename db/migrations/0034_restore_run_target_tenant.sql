-- +goose Up
-- `restore_run.tenant_id` carries a comment calling it "the target tenant", and row level security
-- compares it against `current_tenant_id()`. For five of the six modes of backup-restore.md §8.2
-- those are the same tenant and nothing is missing. For NEW_TENANT they are not: the archive is
-- imported beside the living data as a tenant of its own, and a run row filed under the tenant that
-- did not exist a moment ago is a run the person who asked for it cannot read - the `result_url`
-- they were handed answers 404.
--
-- So the row belongs to the tenant that *asked*, which is the one it has to be visible in, and the
-- tenant being restored *into* gets a column of its own. They differ only for NEW_TENANT.
--
-- Expand only, and nullable: an INSTANCE restore is restoring into no tenant at all.
ALTER TABLE restore_run ADD COLUMN target_tenant_id uuid;

COMMENT ON COLUMN restore_run.tenant_id IS
  'The tenant that asked for the restore, and the one the row is visible in (RLS).';
COMMENT ON COLUMN restore_run.target_tenant_id IS
  'The tenant being restored into. Differs from tenant_id only for NEW_TENANT, and is NULL for an instance restore.';

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE restore_run DROP COLUMN target_tenant_id;
