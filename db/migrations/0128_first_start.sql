-- The first start, and the way back in (UC-INS-01, UC-INS-02, UC-INS-03, UC-INS-12).
--
-- **A setup code.** A fresh installation - no workspace, nobody in the operator register - prints a
-- one-time code at start, and whoever holds it creates the first workspace, its owner and the
-- owner's operator entry in the browser. Only the code's hash is stored, under the installation
-- secret's own purpose label, for thirty minutes. Every process that starts on an empty
-- installation issues its own code, so several may be valid at once; the claim below serialises
-- the setups and the first one wins.
--
-- `instance_setup_code` belongs to no workspace and carries no row-level policy - the operator
-- register's precedent, for the same reason: the application role has no grant on it at all, and
-- it is reached only through the functions below. The exception is entered where the others are:
-- `db/schema.sql`, the boundary integration test and the restore drill's own list.
--
-- **Empty means no tenant row at all.** A workspace pending deletion, or one soft-deleted, is still
-- a workspace somebody ran: an installation is set up once, and the way back after that is the
-- machine's (`hubtask operator add`), never a public page.
--
-- **The census stops counting a workspace that holds no person.** The scripts used to create a
-- workspace whose only account was a service account, to hold the control plane's first credential.
-- It is not a customer, and the overview no longer counts it as one; it is reported as a number of
-- its own, so the screen still adds up with the workspace list. Nothing here deletes it: a
-- workspace holds data, and removing one is not a migration's decision.
--
-- **The register's state for the health report.** Two numbers - workspaces and operators - so the
-- report can say when an installation hosting several workspaces has nobody registered to run it,
-- counted exactly as `is_operator` counts.

-- +goose Up

CREATE TABLE instance_setup_code (
  id         uuid PRIMARY KEY,
  code_hash  bytea NOT NULL UNIQUE,
  issued_at  timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);
REVOKE ALL ON instance_setup_code FROM PUBLIC;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION installation_unset() RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT NOT EXISTS (SELECT 1 FROM tenant) AND NOT EXISTS (SELECT 1 FROM operator)
$$;
-- +goose StatementEnd

-- An issue lands only on an empty installation, and clears the codes that have run out on the way:
-- a restart is how an expired code is renewed, and the old rows are nothing but noise.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION issue_setup_code(
  p_id uuid, p_hash bytea, p_issued timestamptz, p_expires timestamptz
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE issued integer;
BEGIN
  IF NOT installation_unset() THEN
    RETURN false;
  END IF;
  DELETE FROM instance_setup_code WHERE expires_at <= p_issued;
  INSERT INTO instance_setup_code (id, code_hash, issued_at, expires_at)
  VALUES (p_id, p_hash, p_issued, p_expires)
  ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS issued = ROW_COUNT;
  RETURN issued > 0;
END $$;
-- +goose StatementEnd

-- The claim, inside the setup's own transaction. A transaction-scoped advisory lock serialises two
-- setups with two valid codes; the emptiness is read after the lock, so the second waits for the
-- first to commit and then finds a workspace. Spending a code deletes every code: one setup per
-- installation. The answer is one word, never a row.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION claim_setup_code(p_hash bytea, p_now timestamptz) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE expires timestamptz;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtext('hubtask.installation_setup'));
  IF NOT installation_unset() THEN
    RETURN 'ALREADY_DONE';
  END IF;
  SELECT expires_at INTO expires FROM instance_setup_code WHERE code_hash = p_hash;
  IF NOT FOUND THEN
    RETURN 'WRONG';
  END IF;
  IF expires <= p_now THEN
    RETURN 'EXPIRED';
  END IF;
  DELETE FROM instance_setup_code;
  RETURN 'CLAIMED';
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION operator_register_state()
RETURNS TABLE (workspaces bigint, operators bigint)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT (SELECT count(*) FROM tenant), (SELECT count(*) FROM operator)
$$;
-- +goose StatementEnd

-- A bare address in single mode: the installation's one workspace, whose short name the person
-- setting it up never chose. Nothing when there is not exactly one.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION resolve_single_operator_account(p_email text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT a.id
  FROM account a
  WHERE a.tenant_id = resolve_tenant(NULL)
    AND lower(a.email) = lower(btrim(p_email))
    AND a.deleted_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

-- The census gains a sixth number, so its return type changes and it has to be dropped first. In
-- one transaction with its new definition, so no caller ever meets it missing (0118's precedent);
-- the query names its columns, so a pod still running the previous release reads the new one.
DROP FUNCTION instance_census();

-- +goose StatementBegin
CREATE FUNCTION instance_census()
RETURNS TABLE (
  workspaces_active           bigint,
  workspaces_suspended        bigint,
  workspaces_pending_deletion bigint,
  accounts_active             bigint,
  accounts_total              bigint,
  workspaces_service_only     bigint
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  WITH customer AS (
    SELECT t.status
    FROM tenant t
    WHERE EXISTS (
      SELECT 1 FROM account a
      WHERE a.tenant_id = t.id AND a.kind = 'USER' AND a.deleted_at IS NULL
    )
  )
  SELECT
    (SELECT count(*) FROM customer WHERE status = 'ACTIVE'),
    (SELECT count(*) FROM customer WHERE status = 'SUSPENDED'),
    (SELECT count(*) FROM customer WHERE status = 'PENDING_DELETION'),
    (SELECT count(*) FROM account WHERE deleted_at IS NULL AND status = 'ACTIVE'),
    (SELECT count(*) FROM account WHERE deleted_at IS NULL),
    (SELECT count(*) FROM tenant) - (SELECT count(*) FROM customer)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $grant$
BEGIN
  EXECUTE 'REVOKE ALL ON FUNCTION installation_unset() FROM PUBLIC';
  EXECUTE 'REVOKE ALL ON FUNCTION issue_setup_code(uuid, bytea, timestamptz, timestamptz) FROM PUBLIC';
  EXECUTE 'REVOKE ALL ON FUNCTION claim_setup_code(bytea, timestamptz) FROM PUBLIC';
  EXECUTE 'REVOKE ALL ON FUNCTION operator_register_state() FROM PUBLIC';
  EXECUTE 'REVOKE ALL ON FUNCTION resolve_single_operator_account(text) FROM PUBLIC';
  EXECUTE 'REVOKE ALL ON FUNCTION instance_census() FROM PUBLIC';
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hubtask_app') THEN
    EXECUTE 'GRANT EXECUTE ON FUNCTION installation_unset() TO hubtask_app';
    EXECUTE 'GRANT EXECUTE ON FUNCTION issue_setup_code(uuid, bytea, timestamptz, timestamptz) TO hubtask_app';
    EXECUTE 'GRANT EXECUTE ON FUNCTION claim_setup_code(bytea, timestamptz) TO hubtask_app';
    EXECUTE 'GRANT EXECUTE ON FUNCTION operator_register_state() TO hubtask_app';
    EXECUTE 'GRANT EXECUTE ON FUNCTION resolve_single_operator_account(text) TO hubtask_app';
    EXECUTE 'GRANT EXECUTE ON FUNCTION instance_census() TO hubtask_app';
  END IF;
END $grant$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
