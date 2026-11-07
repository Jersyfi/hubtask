-- An SLA policy may name a calendar the installation offers (UC-SVC-01 check 6): the instance
-- file's `business_calendars` are no rows of the workspace, so a foreign key to `business_calendar`
-- refused every policy naming one. The use cases hold the key instead - a policy names a calendar
-- the workspace or the installation holds, and a workspace calendar a policy names is not deleted
-- unless the installation offers one of the same key.
--
-- Expand only (rule 12): a constraint is dropped, nothing is. The previous binary keeps refusing
-- an unknown key by its own mapping of the violation that no longer comes, so until it is replaced
-- it may store a key nobody holds; the clocks then refuse to arm on it, as on a deleted calendar.
-- +goose Up
ALTER TABLE sla_policy DROP CONSTRAINT IF EXISTS sla_policy_calendar_fkey;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
