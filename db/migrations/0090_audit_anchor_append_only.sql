-- The audit anchors are written now (A-2, P-13, audit.md §3), and they are append-only.
--
-- `audit_anchor` has waited for its writer since 0001_init and inherited the default grants
-- while it waited. An anchor is the chain's end exported outside the database - the one thing
-- that says anything against somebody who can rewrite the whole trail - and the row that says
-- where it went and what the object's digest was must not be one the application can rewrite or
-- remove. The same discipline the trail and the pseudonyms have: no UPDATE, no DELETE, no
-- TRUNCATE for the application role; the row dies with the tenant through its foreign key, as the
-- data catalogue has said all along (`IMMUTABLE`). Expand only: a grant narrowed for a table no
-- previous version writes.

-- +goose Up

REVOKE UPDATE, DELETE, TRUNCATE ON audit_anchor FROM hubtask_app;
GRANT  SELECT, INSERT ON audit_anchor TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
