-- A notification can name the role somebody was given (UC-NOT-01 check 9): the grant is its
-- subject, and the mail reads the role, the scope and the place's name from it at delivery, so the
-- record holds an identifier and no content.
--
-- This migration prepares the reference; 0145 adds the key. `membership` has had no unique index on
-- `(tenant_id, id)`, which a tenant-scoped foreign key needs (the reference schema's header), and the
-- cascade a revocation sets off needs an index on the referencing side or it scans `notification`.
-- Both are built CONCURRENTLY on live tables, so no grant and no notification waits while they build
-- - which is why this migration takes no transaction. IF NOT EXISTS covers the retry after an
-- interrupted build, which leaves an invalid index behind under the same name.
--
-- Expand only (rule 12): a nullable column nothing of the previous release selects or writes, and
-- two indexes. A pod of the previous release inserts no row naming a grant.

-- +goose NO TRANSACTION

-- +goose Up

ALTER TABLE notification ADD COLUMN IF NOT EXISTS membership_id uuid;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS membership_tenant_id_id_key
  ON membership (tenant_id, id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_membership_idx
  ON notification (tenant_id, membership_id)
  WHERE membership_id IS NOT NULL;

-- +goose Down

-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
