-- A portal organisation's list joins its requests to the people at its mail domains in the query
-- (UC-SVC-11 check 3) instead of reading a bounded list of accounts first: this index finds a
-- workspace's people by their address's domain.
--
-- Forward-only and safe for a rolling update: an index only, CONCURRENTLY, outside a transaction,
-- IF NOT EXISTS for an interrupted build (drop the invalid index and run again). Nothing older
-- reads it.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS account_mail_domain_idx
  ON account (tenant_id, lower(split_part(email, '@', 2)))
  WHERE email IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
