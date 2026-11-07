-- A hub's own business calendar (UC-SVC-01 check 5, configuration model *Calendar*: installation,
-- workspace, hub): the hub's working hours and holidays under the key of one of the workspace's
-- calendars, which an SLA policy naming that key counts on for the hub's entries. The workspace's
-- calendar stays the policy's reference (`sla_policy_calendar_fkey`), so a hub's calendar names a
-- key the workspace holds, and a workspace calendar a hub keeps its own version of is in use.
--
-- Expand only (rule 12): a new table nothing of the previous release reads or writes.
-- +goose Up
CREATE TABLE IF NOT EXISTS hub_business_calendar (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id      uuid NOT NULL,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  spec        jsonb NOT NULL,
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT hub_business_calendar_key_uq UNIQUE (tenant_id, hub_id, key),
  CONSTRAINT hub_business_calendar_hub_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT hub_business_calendar_key_fkey
    FOREIGN KEY (tenant_id, key) REFERENCES business_calendar (tenant_id, key)
);
-- The workspace calendar's deletion asks this side whether a hub keeps its own version.
CREATE INDEX IF NOT EXISTS hub_business_calendar_key_idx ON hub_business_calendar (tenant_id, key);
ALTER TABLE hub_business_calendar ENABLE ROW LEVEL SECURITY;
ALTER TABLE hub_business_calendar FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON hub_business_calendar;
CREATE POLICY tenant_isolation ON hub_business_calendar
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON hub_business_calendar TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
