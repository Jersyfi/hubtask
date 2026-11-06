-- The workspace's sample content (user-experience.md §3.6): the mark that a showcase or a person
-- made an object to look at, so that one act removes it all. The object stays an ordinary object
-- of its kind; the mark only names it. `object_id` names a row of the table its kind says, so it
-- carries no foreign key - a mark whose object is gone counts as removed. `version` is the
-- object's version as it was made: a kind without a trash is deleted only while it still stands
-- there.
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS sample_object (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_.]{0,99}$'),
  kind       text NOT NULL CHECK (kind IN ('CONTAINER', 'ENTRY', 'PAGE', 'WHITEBOARD', 'GOAL', 'PORTFOLIO', 'DASHBOARD', 'TIME_RECORD')),
  object_id  uuid NOT NULL,
  version    bigint CHECK (version >= 0),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, key)
);

ALTER TABLE sample_object ENABLE ROW LEVEL SECURITY;
ALTER TABLE sample_object FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON sample_object;
CREATE POLICY tenant_isolation ON sample_object
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sample_object TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
