-- The passphrase a restore of a passphrase-encrypted archive was asked with (backup-restore.md §4):
-- sealed under the installation's master key and bound to its run, held only while the run goes -
-- the job that reads the archive runs minutes later, in another process - and emptied by the
-- statement that ends the run, whatever its outcome.
--
-- Expand only (rule 12): two nullable columns nothing older reads or writes.

-- +goose Up
ALTER TABLE restore_run ADD COLUMN IF NOT EXISTS passphrase_enc bytea;
ALTER TABLE restore_run ADD COLUMN IF NOT EXISTS passphrase_key_id text;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
