-- How satisfied a requester was (M5, UC-SVC-07): one score from 1 to 5 per resolved request,
-- replaced when they rate again. It names nobody - the rater is who raised the request, and the
-- words they add are a comment of theirs. A new table, so expand only (versioning-release.md §4).
-- +goose Up
CREATE TABLE IF NOT EXISTS satisfaction_rating (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL,
  score      smallint NOT NULL CHECK (score BETWEEN 1 AND 5),
  rated_at   timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT satisfaction_rating_item_uq UNIQUE (tenant_id, item_id),
  CONSTRAINT satisfaction_rating_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE satisfaction_rating ENABLE ROW LEVEL SECURITY;
ALTER TABLE satisfaction_rating FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON satisfaction_rating;
CREATE POLICY tenant_isolation ON satisfaction_rating
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON satisfaction_rating TO hubtask_app;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
