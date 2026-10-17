-- Where each read of the rule-driven retention sweep stopped, so the next pass continues there
-- (data-retention.md §5).
--
-- A pass reads past what it cannot act on, page by page, inside the job's deadline. A workspace with
-- more such entries than one deadline can read would fail every pass and start again at the first
-- page; so each read stops at a budget and keeps its place here, and every due entry is reached
-- within a bounded number of passes. One row per reader: phase one of a kind (`ANNOUNCE`, by
-- `data_kind`), a chain's second stage (`CHAIN`, by its rule, removed with the rule) and phase two
-- (`ACT`). The position is the order the candidate statements read in - the anchor's moment and the
-- entry's identifier - and nothing else: no title, no condition, no value of an entry (rule 10).
-- No foreign key to the entry: a position stays valid when the entry it names is gone.
--
-- Under the tenant's row security like every tenant table (rule 3), and its rule referenced on the
-- workspace and the rule together (ADR-0024). Not in an archive: a restore
-- starts at the beginning, which is always a correct place to start.
--
-- Expand only: a new table nothing older reads or writes (rule 12).

-- +goose Up
CREATE TABLE IF NOT EXISTS retention_cursor (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  reader     text NOT NULL CHECK (reader IN ('ANNOUNCE','CHAIN','ACT')),
  data_kind  text,
  rule_id    uuid,
  after_at   timestamptz NOT NULL,
  after_id   uuid NOT NULL,
  CONSTRAINT retention_cursor_reader_shape CHECK (
    (reader = 'ANNOUNCE' AND data_kind IS NOT NULL AND rule_id IS NULL)
    OR (reader = 'CHAIN' AND data_kind IS NULL AND rule_id IS NOT NULL)
    OR (reader = 'ACT' AND data_kind IS NULL AND rule_id IS NULL)
  ),
  -- On the workspace and the rule together, so no row can name another workspace's rule (ADR-0024;
  -- the key is 0172's index).
  CONSTRAINT retention_cursor_rule_fkey FOREIGN KEY (tenant_id, rule_id)
    REFERENCES retention_rule (tenant_id, id) ON DELETE CASCADE
);

-- One place per reader. `NULLS NOT DISTINCT`, because two of the three readers leave a column
-- NULL, and two NULLs must name the same reader. Built without CONCURRENTLY: the table was created
-- in this same migration and nobody reads it yet.
CREATE UNIQUE INDEX IF NOT EXISTS retention_cursor_reader_uq
  ON retention_cursor (tenant_id, reader, data_kind, rule_id) NULLS NOT DISTINCT;
CREATE INDEX IF NOT EXISTS retention_cursor_rule_idx
  ON retention_cursor (tenant_id, rule_id) WHERE rule_id IS NOT NULL;

ALTER TABLE retention_cursor ENABLE ROW LEVEL SECURITY;
ALTER TABLE retention_cursor FORCE ROW LEVEL SECURITY;
-- +goose StatementBegin
DO $policy$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'retention_cursor' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON retention_cursor
      USING (tenant_id = current_tenant_id())
      WITH CHECK (tenant_id = current_tenant_id());
  END IF;
END $policy$;
-- +goose StatementEnd

GRANT SELECT, INSERT, UPDATE, DELETE ON retention_cursor TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration';
END $forward_only$;
-- +goose StatementEnd
