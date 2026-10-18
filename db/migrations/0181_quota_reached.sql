-- A limit reached, marked once per crossing (UC-INS-14 check 2, multi-tenancy.md §4.3).
--
-- A workspace's create refused at a capacity limit marks the limit at its value here, and the
-- installation event `limit_reached` is written only by the call that set the mark; a create of that
-- quota admitted later removes it, so the next refusal is the next crossing, and a changed limit is
-- one too. The AI budget marks its day, so a budget spent is told once a day. `period` holds the
-- limit's value or the day; marking one forgets the quota's others. The insert is `ON CONFLICT DO
-- NOTHING`: two refusals or two meterings at once mark once.
--
-- A workspace table under the ordinary tenant policy, and gone with the workspace.
--
-- Expand only and safe for a rolling update (rule 12): a new table that the previous release does
-- not know.

-- +goose Up
CREATE TABLE IF NOT EXISTS quota_reached (
  tenant_id  uuid NOT NULL REFERENCES tenant (id) ON DELETE CASCADE,
  quota      text NOT NULL CHECK (quota ~ '^[a-z][a-z0-9_]*$'),
  period     text NOT NULL DEFAULT '',
  reached_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, quota, period)
);

ALTER TABLE quota_reached ENABLE ROW LEVEL SECURITY;
ALTER TABLE quota_reached FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'quota_reached' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON quota_reached
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, DELETE ON quota_reached TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
