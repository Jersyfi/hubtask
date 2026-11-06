-- Live documents (F-8, offline-sync.md §13): each document's CRDT update log, the snapshots that
-- compact it, and its head. The server stores and orders the updates and never reads them; the
-- format is the editors' CRDT update.
--
-- `realtime_document` is the head: the last place given and the one the newest snapshot covers,
-- locked by the append that takes the next place, so two writers of one document never share a
-- place. A document's kind is what it belongs to (a page, a whiteboard); its identifier is the
-- owner's, so there is no foreign key - the owning kind erases its documents with itself.
--
-- A snapshot marks the updates it covers `compacted_at` and the snapshot it replaces
-- `superseded_at`; neither is removed by the write. The DOCUMENT_HISTORY retention kind removes
-- what is compacted or superseded once its period has passed (data-retention.md §3). An author
-- erased leaves the update without a name.
--
-- Every insert into the log rings the change stream's doorbell (migration 0022): a connection on
-- another process is woken by the tenant and reads the log through the transaction wrapper.
--
-- Expand only: new tables nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS realtime_document (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  kind         text NOT NULL CHECK (kind ~ '^[a-z][a-z_]{1,31}$'),
  document_id  uuid NOT NULL,
  head_seq     bigint NOT NULL DEFAULT 0 CHECK (head_seq >= 0),
  snapshot_seq bigint NOT NULL DEFAULT 0 CHECK (snapshot_seq >= 0 AND snapshot_seq <= head_seq),
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, kind, document_id)
);

CREATE TABLE IF NOT EXISTS realtime_update (
  tenant_id    uuid NOT NULL,
  kind         text NOT NULL,
  document_id  uuid NOT NULL,
  seq          bigint NOT NULL CHECK (seq > 0),
  data         bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 1048576),
  author_id    uuid,
  created_at   timestamptz NOT NULL,
  compacted_at timestamptz,
  PRIMARY KEY (tenant_id, kind, document_id, seq),
  CONSTRAINT realtime_update_document_fkey FOREIGN KEY (tenant_id, kind, document_id)
    REFERENCES realtime_document (tenant_id, kind, document_id) ON DELETE CASCADE,
  CONSTRAINT realtime_update_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
-- The retention sweep's pass: what a snapshot covers, by when.
CREATE INDEX IF NOT EXISTS realtime_update_compacted_idx
  ON realtime_update (tenant_id, compacted_at) WHERE compacted_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS realtime_snapshot (
  tenant_id     uuid NOT NULL,
  kind          text NOT NULL,
  document_id   uuid NOT NULL,
  up_to_seq     bigint NOT NULL CHECK (up_to_seq > 0),
  state         bytea NOT NULL CHECK (octet_length(state) BETWEEN 1 AND 16777216),
  created_at    timestamptz NOT NULL,
  superseded_at timestamptz,
  PRIMARY KEY (tenant_id, kind, document_id, up_to_seq),
  CONSTRAINT realtime_snapshot_document_fkey FOREIGN KEY (tenant_id, kind, document_id)
    REFERENCES realtime_document (tenant_id, kind, document_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS realtime_snapshot_superseded_idx
  ON realtime_snapshot (tenant_id, superseded_at) WHERE superseded_at IS NOT NULL;

ALTER TABLE realtime_document ENABLE ROW LEVEL SECURITY;
ALTER TABLE realtime_document FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON realtime_document;
CREATE POLICY tenant_isolation ON realtime_document
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON realtime_document TO hubtask_app;

ALTER TABLE realtime_update ENABLE ROW LEVEL SECURITY;
ALTER TABLE realtime_update FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON realtime_update;
CREATE POLICY tenant_isolation ON realtime_update
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON realtime_update TO hubtask_app;

ALTER TABLE realtime_snapshot ENABLE ROW LEVEL SECURITY;
ALTER TABLE realtime_snapshot FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON realtime_snapshot;
CREATE POLICY tenant_isolation ON realtime_snapshot
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON realtime_snapshot TO hubtask_app;

DROP TRIGGER IF EXISTS realtime_update_notify ON realtime_update;
CREATE TRIGGER realtime_update_notify
  AFTER INSERT ON realtime_update
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_change();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
