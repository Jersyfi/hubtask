-- An internal note beside the public replies (M5, UC-SVC-04): a comment the team writes for
-- itself, which a requester following their request through the portal never reads. Expand only:
-- a column with a default, so the previous version writes public comments as it always did and
-- an archive without the column restores them public (versioning-release.md §4).
-- +goose Up
ALTER TABLE comment ADD COLUMN IF NOT EXISTS internal boolean NOT NULL DEFAULT false;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
