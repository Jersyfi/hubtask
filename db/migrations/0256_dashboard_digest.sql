-- A dashboard's digest by mail (UC-GOL-04): one person's subscription to one dashboard, at an
-- hour of their zone, daily or on one weekday. The mail is computed as that person when it is
-- sent, so no number is stored here. A new table only, so expand only (versioning-release.md §4).
--
-- `account_id` names its subscriber without a foreign key, as the time module's rows do; a
-- deleted account's digest stops at its next run. The dashboard's deletion takes its digests.
-- +goose Up
CREATE TABLE IF NOT EXISTS dashboard_digest (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  dashboard_id uuid NOT NULL REFERENCES dashboard(id) ON DELETE CASCADE,
  account_id   uuid NOT NULL,
  cadence      text NOT NULL CHECK (cadence IN ('DAILY', 'WEEKLY')),
  weekday      smallint NOT NULL DEFAULT 0 CHECK (weekday BETWEEN 0 AND 7),
  hour         smallint NOT NULL CHECK (hour BETWEEN 0 AND 23),
  time_zone    text NOT NULL CHECK (char_length(time_zone) BETWEEN 1 AND 64),
  next_at      timestamptz NOT NULL,
  last_sent_at timestamptz,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  CHECK ((cadence = 'WEEKLY') = (weekday > 0)),
  UNIQUE (tenant_id, dashboard_id, account_id)
);
CREATE INDEX IF NOT EXISTS dashboard_digest_account_idx ON dashboard_digest (tenant_id, account_id);
ALTER TABLE dashboard_digest ENABLE ROW LEVEL SECURITY;
ALTER TABLE dashboard_digest FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON dashboard_digest;
CREATE POLICY tenant_isolation ON dashboard_digest
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON dashboard_digest TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
