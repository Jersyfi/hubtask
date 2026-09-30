-- What an operator needs to see the installation at a glance (SI-17, ADR-0070 §5).
--
-- **Counts, states and limits - never rows.** That is ADR-0070 §5 in its own words, and this
-- function is the shape of it: one row of numbers, and no way to ask it for anybody's data. The
-- dashboard's overview is the whole of its caller.
--
-- It has to be `SECURITY DEFINER` for the same reason `is_operator` does: `account` is behind row
-- level security and `FORCE`, so the application role cannot count across workspaces at all - and
-- the count is the one thing an operator legitimately needs that the boundary refuses. Narrow by
-- construction is what makes that acceptable: the function returns five integers, so a caller that
-- wanted rows would have to change the function, which is a migration somebody reviews.
--
-- `tenant` is counted through the same function rather than through the application role's own read,
-- although `/admin/tenants` already enumerates workspaces: two counts of one thing drift, and the
-- overview's numbers have to agree with each other on the same instant - which one statement gives
-- and two do not.
--
-- The exception is entered where the others are: the function is granted explicitly, revoked from
-- PUBLIC, and reachable only through the control plane's use case behind `admin:tenants` and the
-- operator register.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION instance_census()
RETURNS TABLE (
  workspaces_active           bigint,
  workspaces_suspended        bigint,
  workspaces_pending_deletion bigint,
  accounts_active             bigint,
  accounts_total              bigint
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT
    (SELECT count(*) FROM tenant WHERE status = 'ACTIVE'),
    (SELECT count(*) FROM tenant WHERE status = 'SUSPENDED'),
    (SELECT count(*) FROM tenant WHERE status = 'PENDING_DELETION'),
    -- Live people, which is what "how big is this installation" means: an account that was deleted
    -- is gone as far as anybody operating the installation is concerned.
    (SELECT count(*) FROM account WHERE deleted_at IS NULL AND status = 'ACTIVE'),
    (SELECT count(*) FROM account WHERE deleted_at IS NULL)
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION instance_census() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION instance_census() TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
