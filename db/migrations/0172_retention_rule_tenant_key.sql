-- The key a tenant-scoped reference to a retention rule points at (ADR-0024): `retention_cursor`
-- (0173) names a chain's rule, and a reference on `rule_id` alone could cross workspaces.
--
-- `retention_rule` has had no unique index on `(tenant_id, id)`. It is built CONCURRENTLY on the live
-- table, so no rule change waits while it builds - which is why this migration takes no
-- transaction. IF NOT EXISTS covers the retry after an interrupted build, which leaves an invalid
-- index behind under the same name; 0173's key then fails loudly rather than resting on it.
--
-- Expand only (rule 12): an index, which a pod of the previous release neither knows nor needs.

-- +goose NO TRANSACTION

-- +goose Up

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS retention_rule_tenant_id_uq
  ON retention_rule (tenant_id, id);

-- +goose Down

-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
