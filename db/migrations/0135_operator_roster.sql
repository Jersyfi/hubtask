-- The operator register a person can read and safely change (UC-INS-05, UC-INS-06).
--
-- **A roster with names.** `operator_register()` answers identifiers, and an identifier is not
-- something an operator can look up: `account` is behind row level security, so the screen showed
-- two UUIDs per row. `operator_roster(caller)` answers the register's rows with the account's name,
-- address or sign-in name and kind, its workspace's short name and name, and the adder's name. It
-- names **only accounts the register holds** - never a workspace's other accounts - and answers no
-- row at all unless `is_operator(caller)`, so the application role cannot use it to learn more than
-- the control plane's own use case shows to an operator.
--
-- **Adding cannot lock the adder out.** On an empty register the only workspace's owner is the
-- operator; the first row turns that off. `enlist_operator(account, by)` therefore adds the caller
-- as well when the register was empty, under a lock on the table so that two first additions cannot
-- both read "empty". It adds only an `ACTIVE`, undeleted account, and answers what happened as a
-- word: ADDED, ADDED_WITH_CALLER, PRESENT, NOT_ACTIVE or UNKNOWN.
--
-- **Removing tells "not there" from "the last one".** `dismiss_operator(account)` keeps the last
-- operator in the statement, as `drop_operator` does, and answers REMOVED, LAST or ABSENT.
--
-- Expand only (rule 12): `add_operator` and `drop_operator` stay for an instance of the previous
-- release running beside this one. A release after this one drops them, once nothing calls them.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION operator_roster(p_caller uuid)
RETURNS TABLE (
  tenant_id      uuid,
  account_id     uuid,
  added_at       timestamptz,
  added_by       uuid,
  display_name   text,
  email          text,
  sign_in_name   text,
  kind           text,
  workspace_slug text,
  workspace_name text,
  added_by_name  text
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT o.tenant_id, o.account_id, o.added_at, o.added_by,
         a.display_name, a.email, a.sign_in_name, a.kind::text,
         t.slug, t.display_name,
         b.display_name
  FROM operator o
  JOIN account a ON a.tenant_id = o.tenant_id AND a.id = o.account_id
  JOIN tenant t ON t.id = o.tenant_id
  LEFT JOIN account b ON b.id = o.added_by AND b.deleted_at IS NULL
  WHERE is_operator(p_caller)
  ORDER BY o.added_at, o.account_id
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enlist_operator(p_account uuid, p_by uuid) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE
  target_status account_status;
  was_empty     boolean;
  added         integer;
  caller_added  integer := 0;
BEGIN
  -- Conflicts with itself and with every insert and delete, so "the register is empty" is read and
  -- written under one lock: two first additions at once leave one empty-register branch, not two.
  LOCK TABLE operator IN SHARE ROW EXCLUSIVE MODE;

  SELECT a.status INTO target_status
  FROM account a WHERE a.id = p_account AND a.deleted_at IS NULL;
  IF NOT FOUND THEN
    RETURN 'UNKNOWN';
  END IF;
  IF target_status <> 'ACTIVE' THEN
    RETURN 'NOT_ACTIVE';
  END IF;

  was_empty := NOT EXISTS (SELECT 1 FROM operator);

  INSERT INTO operator (tenant_id, account_id, added_by)
  SELECT a.tenant_id, a.id, p_by FROM account a WHERE a.id = p_account
  ON CONFLICT (tenant_id, account_id) DO NOTHING;
  GET DIAGNOSTICS added = ROW_COUNT;
  IF added = 0 THEN
    RETURN 'PRESENT';
  END IF;

  -- The adder was an operator only because the register was empty; the row just written ends that.
  IF was_empty AND p_by IS NOT NULL AND p_by <> p_account THEN
    INSERT INTO operator (tenant_id, account_id, added_by)
    SELECT a.tenant_id, a.id, p_by
    FROM account a
    WHERE a.id = p_by AND a.deleted_at IS NULL AND a.status = 'ACTIVE'
    ON CONFLICT (tenant_id, account_id) DO NOTHING;
    GET DIAGNOSTICS caller_added = ROW_COUNT;
  END IF;

  IF caller_added > 0 THEN
    RETURN 'ADDED_WITH_CALLER';
  END IF;
  RETURN 'ADDED';
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION dismiss_operator(p_account uuid) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE removed integer;
BEGIN
  -- The last one stays, in the statement: two operators removing each other at once would both read
  -- "there are two".
  DELETE FROM operator
  WHERE account_id = p_account
    AND (SELECT count(*) FROM operator) > 1;
  GET DIAGNOSTICS removed = ROW_COUNT;
  IF removed > 0 THEN
    RETURN 'REMOVED';
  END IF;
  IF EXISTS (SELECT 1 FROM operator WHERE account_id = p_account) THEN
    RETURN 'LAST';
  END IF;
  RETURN 'ABSENT';
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION operator_roster(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION enlist_operator(uuid, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION dismiss_operator(uuid) FROM PUBLIC;

-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hubtask_app') THEN
    GRANT EXECUTE ON FUNCTION operator_roster(uuid) TO hubtask_app;
    GRANT EXECUTE ON FUNCTION enlist_operator(uuid, uuid) TO hubtask_app;
    GRANT EXECUTE ON FUNCTION dismiss_operator(uuid) TO hubtask_app;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
