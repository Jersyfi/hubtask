-- An inventory object's own history (UC-AST-01, IN-2): one row per thing done to the object - made,
-- changed by hand or by a source, its stage, owner, location or protection need, related and
-- unrelated, confirmed and attested, registered and moved, a file put on or taken off - written in
-- the transaction of the write it records. It is the product's record and not the audit trail:
-- readable by whoever reads the hub, deleted with the object. The change set holds field names,
-- the values of stages, people, places, objects and files, and never an attribute's value, which
-- the object itself shows. Archived with the object (tenant-export.md §6).
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS cmdb_ci_history (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  ci_id       uuid NOT NULL,
  actor_type  text NOT NULL CHECK (actor_type IN ('USER', 'SERVICE_ACCOUNT', 'AUTOMATION', 'AI_AGENT', 'SYSTEM')),
  actor_id    uuid,
  verb        text NOT NULL CHECK (char_length(verb) BETWEEN 1 AND 64),
  change_set  jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(change_set) = 'object'),
  occurred_at timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_ci_history_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
-- A page is read newest first within one object and continues after a boundary.
CREATE INDEX IF NOT EXISTS cmdb_ci_history_page_idx ON cmdb_ci_history (tenant_id, ci_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS cmdb_ci_history_actor_idx ON cmdb_ci_history (tenant_id, actor_id) WHERE actor_id IS NOT NULL;

ALTER TABLE cmdb_ci_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci_history FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_ci_history;
CREATE POLICY tenant_isolation ON cmdb_ci_history
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON cmdb_ci_history TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
