-- A hub's own calendar may keep a version of a calendar the installation offers (UC-SVC-01
-- check 5, second answers 7): the instance file's `business_calendars` are no rows of the
-- workspace, so the foreign key to `business_calendar` refused a hub's version of one unless the
-- workspace created a calendar of the same key - which then replaced the installation's for every
-- other hub. The use cases hold the key instead, as migration 0291 did for `sla_policy`: a hub's
-- calendar names a key the workspace or the installation holds, and a workspace calendar a hub
-- keeps a version of is not deleted unless the installation offers one of the same key.
--
-- Expand only (rule 12): a constraint is dropped, nothing is. The previous binary may, until it is
-- replaced, delete a workspace calendar a hub keeps a version of; the hub's entries then count on
-- the hub's own version, which the clocks read first.
-- +goose Up
ALTER TABLE hub_business_calendar DROP CONSTRAINT IF EXISTS hub_business_calendar_key_fkey;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
