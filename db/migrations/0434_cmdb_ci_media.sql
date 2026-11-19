-- An inventory object's files (UC-AST-01, F-1, IN-2): `cmdb_ci_media` names the media objects an
-- object holds, written by AttachCiMedia and DetachCiMedia once the application has judged the
-- writer at the object's hub (rule 2). It is what lets a reader of the hub read the object
-- (GetMedia) and what the media reconciliation counts. The object's removal - with its hub or the
-- workspace - takes the rows, and the reconciliation then reclaims a file nothing else holds, as
-- it does for an entry's. Archived with the object (tenant-export.md §6).
--
-- Expand only: a new table nothing older reads (rule 12).
-- +goose Up
CREATE TABLE IF NOT EXISTS cmdb_ci_media (
  tenant_id uuid NOT NULL,
  ci_id     uuid NOT NULL,
  media_id  uuid NOT NULL,
  PRIMARY KEY (tenant_id, ci_id, media_id),
  CONSTRAINT cmdb_ci_media_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_ci_media_object_fkey FOREIGN KEY (tenant_id, media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS cmdb_ci_media_object_idx ON cmdb_ci_media (tenant_id, media_id);

ALTER TABLE cmdb_ci_media ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci_media FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON cmdb_ci_media;
CREATE POLICY tenant_isolation ON cmdb_ci_media
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON cmdb_ci_media TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
