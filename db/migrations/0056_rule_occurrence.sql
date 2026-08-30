-- +goose Up
-- What a RELATIVE_DATE rule owes for one entry, and when (G-08, automation.md §1.1).
--
-- "Internally produces occurrence jobs" is what §1.1 says the trigger does, and this is D-02's
-- shape rather than a new one: the `reminder` table has carried "this entry, this moment" since
-- phase 0, and a relative-date rule is the same fact with a rule in place of a person. A row per
-- (rule, entry), moved when the anchor moves and gone when the anchor is cleared.
--
-- It is not a queue. The job is written when the moment arrives; a row here is a moment this
-- tenant owes, which is what lets one poller answer "when do I next owe anything" without a scan.
--
-- A new table, so nothing about a rolling update is at risk: no existing statement reads it, and
-- an old replica that has never heard of it goes on working exactly as it did.
CREATE TABLE IF NOT EXISTS rule_occurrence (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  rule_id    uuid NOT NULL,
  item_id    uuid NOT NULL,
  -- The instant, in UTC like every other timestamp here. The offset was applied where the anchor
  -- was read, so nothing downstream has to know what "24 hours before" meant.
  fire_at    timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  -- Both cascades are deliberate. A deleted rule owes nothing, and a purged entry has no anchor to
  -- measure from - and neither absence is a moment anybody should be woken for. The run log is the
  -- record that outlives them; this table is only what is still owed.
  CONSTRAINT rule_occurrence_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT rule_occurrence_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);

-- One moment per rule per entry. A rule that owed two moments for one entry would fire twice for
-- one deadline, and the upsert that keeps the moment in step with the anchor conflicts on this.
CREATE UNIQUE INDEX IF NOT EXISTS rule_occurrence_uq
  ON rule_occurrence (tenant_id, rule_id, item_id);

-- What the pass asks: which of this tenant's moments have come, oldest first.
CREATE INDEX IF NOT EXISTS rule_occurrence_due_idx ON rule_occurrence (tenant_id, fire_at);

-- Every tenant-scoped table is behind row level security, and this one is no exception: the
-- moments a workspace owes are that workspace's.
ALTER TABLE rule_occurrence ENABLE ROW LEVEL SECURITY;
ALTER TABLE rule_occurrence FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON rule_occurrence
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- The grant is explicit rather than left to the default privileges, because those follow the role
-- that creates the table and a migration is not always applied by the same one.
GRANT SELECT, INSERT, UPDATE, DELETE ON rule_occurrence TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
