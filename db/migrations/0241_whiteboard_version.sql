-- A whiteboard's history (M4, UC-CNV-06): one row per drawing session - the scene as it stood when
-- the session ended - and a named version its own row. Restoring is the client's: it writes the
-- older scene into the live document, and the save that follows is a version of its own (P-03).
--
-- Expand only: a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS whiteboard_version (
  tenant_id     uuid NOT NULL,
  whiteboard_id uuid NOT NULL,
  seq           integer NOT NULL CHECK (seq > 0),
  title         text NOT NULL,
  scene         jsonb NOT NULL CHECK (jsonb_typeof(scene) = 'array' AND jsonb_array_length(scene) <= 5000),
  author_id     uuid,
  name          text NOT NULL DEFAULT '' CHECK (char_length(name) <= 120),
  created_at    timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, whiteboard_id, seq),
  CONSTRAINT whiteboard_version_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT whiteboard_version_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);

ALTER TABLE whiteboard_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_version FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard_version;
CREATE POLICY tenant_isolation ON whiteboard_version
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON whiteboard_version TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
