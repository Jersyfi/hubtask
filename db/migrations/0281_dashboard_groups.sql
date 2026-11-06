-- A dashboard shared with groups (UC-GOL-04): its owner names up to twenty groups whose members see
-- it, each where they read its place - its numbers recomputed for them as for every reader. The
-- groups are named by identifier; a group gone matches nobody.
--
-- Expand only (rule 12): a column with a default every stored row takes and the previous release
-- never writes - its update keeps the column as it is -, and a sharing check widened to the new
-- value, added NOT VALID and then validated. The previous release reads a GROUPS dashboard as one
-- for its owner alone, the answer that shows nobody too much.
-- +goose Up
ALTER TABLE dashboard ADD COLUMN IF NOT EXISTS group_ids uuid[] NOT NULL DEFAULT '{}';
ALTER TABLE dashboard ADD CONSTRAINT dashboard_group_ids_check
  CHECK (cardinality(group_ids) <= 20) NOT VALID;
ALTER TABLE dashboard VALIDATE CONSTRAINT dashboard_group_ids_check;
ALTER TABLE dashboard DROP CONSTRAINT IF EXISTS dashboard_sharing_check;
ALTER TABLE dashboard ADD CONSTRAINT dashboard_sharing_check
  CHECK (sharing IN ('PRIVATE', 'SCOPE', 'GROUPS')) NOT VALID;
ALTER TABLE dashboard VALIDATE CONSTRAINT dashboard_sharing_check;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
