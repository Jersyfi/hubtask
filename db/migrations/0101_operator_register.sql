-- Who operates this installation (ADR-0070 §1).
--
-- `core/application/service/admin/Provision.go` says it in its own words - "Authorisation here is
-- the scope alone" - and every admin use case calls `RequireScope("admin:tenants")`. Minting that
-- scope needs a fresh step-up and **nothing else**, so any account that can pass one can provision,
-- suspend and delete every workspace on the installation. On a private installation that is
-- correct, because the owner *is* the operator. On a platform with a thousand workspaces it is not.
--
-- **In single mode the register is empty and that means the owner.** A private installation changes
-- in no way: one workspace, its owner, nothing to register. The check reads an empty register as
-- "everybody who could already do this", which is exactly what was true before this table existed.
--
-- **A service account may be an operator.** A purchase platform that provisions workspaces needs a
-- credential that does not belong to a person who may leave, and the first day of a platform is the
-- day that becomes true.
--
-- **No row-level policy, and no grant to the application role either.** The rows name accounts, so
-- unlike `instance_setting` this table *is* a person's data, and a policy-free table the
-- application role could read would let every workspace enumerate the installation's operators. It
-- is reachable only through the four functions below, each narrow by construction, each granted
-- explicitly - `resolve_tenant`'s discipline, applied to a table instead of to a lookup. Its
-- exception is entered in all three lists that must agree about the tenant boundary.

-- +goose Up
CREATE TABLE IF NOT EXISTS operator (
  tenant_id  uuid NOT NULL,
  account_id uuid NOT NULL,
  added_at   timestamptz NOT NULL DEFAULT now(),
  -- Who added them. A bare identifier: the account may be gone, and the journal is where the act
  -- is read from anyway.
  added_by   uuid,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT operator_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- The check, narrow by construction: one boolean, never a listing. Asked twice per act - when the
-- scope is minted and when it is exercised - because either alone is a hole.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION is_operator(p_account uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT
    -- An empty register is the private installation: nothing was configured, and the owner is the
    -- operator exactly as they were before this table existed.
    NOT EXISTS (SELECT 1 FROM operator)
    OR EXISTS (SELECT 1 FROM operator WHERE account_id = p_account)
$$;
-- +goose StatementEnd

-- The listing, for the control plane's own screen. Wider than `is_operator` on purpose and bounded
-- by the same thing every other admin operation is: the scope and the register behind it.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION operator_register() RETURNS SETOF operator
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT o.* FROM operator o ORDER BY o.added_at, o.account_id
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION add_operator(p_account uuid, p_by uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE added integer;
BEGIN
  -- The workspace comes from the account rather than from the caller: a pair that could disagree
  -- would be a pair somebody eventually gets wrong, and this function can read what the application
  -- role cannot. An account nobody holds inserts nothing, which the caller reads as "no such
  -- account".
  INSERT INTO operator (tenant_id, account_id, added_by)
  SELECT a.tenant_id, a.id, p_by
  FROM account a
  WHERE a.id = p_account AND a.deleted_at IS NULL
  ON CONFLICT (tenant_id, account_id) DO NOTHING;
  GET DIAGNOSTICS added = ROW_COUNT;
  RETURN added > 0;
END $$;
-- +goose StatementEnd

-- The last operator cannot remove themselves. In the statement rather than in a read-then-write,
-- because two operators removing each other at the same moment would both read "there are two".
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION drop_operator(p_account uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE removed boolean;
BEGIN
  -- The account alone: an identifier is unique across the installation, and a caller that had to
  -- name the workspace too would have to read the register first to find out which one it is.
  DELETE FROM operator
  WHERE account_id = p_account
    AND (SELECT count(*) FROM operator) > 1;
  GET DIAGNOSTICS removed = ROW_COUNT;
  RETURN removed;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION is_operator(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION operator_register() FROM PUBLIC;
REVOKE ALL ON FUNCTION add_operator(uuid, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION drop_operator(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION is_operator(uuid) TO hubtask_app;
GRANT EXECUTE ON FUNCTION operator_register() TO hubtask_app;
GRANT EXECUTE ON FUNCTION add_operator(uuid, uuid) TO hubtask_app;
GRANT EXECUTE ON FUNCTION drop_operator(uuid) TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
