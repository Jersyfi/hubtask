-- An operator is named by address and workspace, not by an identifier nobody can look up.
--
-- `add_operator` takes an account id, and an operator has no way to find one. The tenant boundary
-- is the reason: `account` is behind row level security, so the control plane cannot list accounts
-- across workspaces, and the screen asked somebody to type a UUID they would have to get out of the
-- database by hand. Every other identifier in this product is something a person can read off
-- something else; this one was not.
--
-- What a person does know is the address somebody signs in with, and which workspace they are in.
-- That pair is unique — `account_email_uq` is `(tenant_id, lower(email))` — and resolving it is the
-- same shape as `resolve_tenant`: a narrow door through a boundary the application role may not
-- cross, answering **one identifier and nothing else**.
--
-- **It answers no more than it is asked.** Not a list, not a search, not a name: an address and a
-- workspace in, an account id or nothing out. An operator who mistypes learns that the pair matches
-- nothing, which is what they would have learnt from `add_operator` anyway — and no more than a
-- caller could learn by trying `add_operator` itself, which they may already do. Nothing here
-- widens what the control plane can see.
--
-- The workspace is named by its slug, which is on the screen the operator is already looking at.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION resolve_operator_account(p_slug text, p_email text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT a.id
  FROM account a
  JOIN tenant t ON t.id = a.tenant_id
  WHERE lower(t.slug) = lower(btrim(p_slug))
    AND lower(a.email) = lower(btrim(p_email))
    AND a.deleted_at IS NULL
  LIMIT 1
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $grant$
BEGIN
  -- The application role calls it; the function's own rights are what cross the boundary.
  EXECUTE 'REVOKE ALL ON FUNCTION resolve_operator_account(text, text) FROM PUBLIC';
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hubtask_app') THEN
    EXECUTE 'GRANT EXECUTE ON FUNCTION resolve_operator_account(text, text) TO hubtask_app';
  END IF;
END $grant$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
