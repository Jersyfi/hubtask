-- An empty register means the owner of a **single-workspace** installation, and nothing else
-- (ADR-0070 §1 amended a second time).
--
-- Migration 0106 narrowed the empty-register branch from "every account of every workspace" to
-- "every active OWNER". On the private installation §1 describes — "it has one workspace and its
-- owner is its operator" — those are the same person. On an installation that hosts customers they
-- are not: the walk of the finished screens provisioned a fourth workspace, and the moment its
-- owner became active they would have been an operator of the whole installation, able to suspend
-- and delete every other customer.
--
-- The sentence §1 wrote has "one workspace" in it. The moment there are two it describes nobody,
-- and every way of picking one of them is a guess:
--
--   * "Every owner" is the hole this closes.
--   * "The oldest workspace's owner" sounds deterministic and is not safe: the oldest workspace may
--     have no active owner at all — which was measured, not imagined, on the walk database, where
--     it left the installation with no operator and no way to appoint one.
--
-- So: **exactly one workspace, and its owner.** More than one and the register must be filled,
-- because an installation hosting customers with nobody registered to run it is a misconfiguration
-- rather than a state with a sensible default.
--
-- **This cannot lock anybody out**, and the reason is the order things happen in. Provisioning a
-- second workspace needs `admin:tenants`, which on an empty register only the first workspace's
-- owner can mint. By the time a second workspace exists, somebody *was* an operator and could have
-- registered themselves. An installation that reaches two workspaces with an empty register is one
-- where that was skipped, and the way back is `hubctl` against the database — the same recovery as
-- a mislaid master key, and for the same reason: it is the machine the installation runs on.
--
-- A workspace being deleted is still counted. Its rows are still there, its owner is still real,
-- and a count that skipped it would make "the owner" flicker back into existence for the grace
-- period.

-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION is_operator(p_account uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT
    -- Registered: the whole of it on any installation that configured one.
    EXISTS (SELECT 1 FROM operator WHERE account_id = p_account)
    -- Or the single-workspace installation, where the owner is the operator - exactly as it was
    -- before this table existed, and no further.
    OR (
      NOT EXISTS (SELECT 1 FROM operator)
      AND (SELECT count(*) FROM tenant) = 1
      AND EXISTS (
        SELECT 1
        FROM membership m
        JOIN account a ON a.tenant_id = m.tenant_id AND a.id = m.account_id
        WHERE m.account_id = p_account
          AND m.role = 'OWNER'
          AND m.scope_type = 'TENANT'
          AND a.deleted_at IS NULL
          AND a.status = 'ACTIVE'
      )
    )
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
