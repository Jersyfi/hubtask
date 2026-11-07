-- Who trashed a page or a whiteboard, so that `GET /trash` names the person for them as it does
-- for entries and containers (0070).
--
-- The same pair as 0070 and for the same reasons: a kind and a nullable identifier, and both
-- nullable for good - a page or board trashed before this migration never learns who did it, and
-- the audit trail stands behind another permission. No CHECK naming the kinds (0070).
--
-- Expand only: two nullable columns on two tables; the previous version neither writes nor reads
-- them, and a restore it performs leaves them standing on a row that is no longer trashed, which
-- the trash listing never reads.

-- +goose Up

ALTER TABLE page ADD COLUMN IF NOT EXISTS deleted_by_type text;
ALTER TABLE page ADD COLUMN IF NOT EXISTS deleted_by_id uuid;

ALTER TABLE whiteboard ADD COLUMN IF NOT EXISTS deleted_by_type text;
ALTER TABLE whiteboard ADD COLUMN IF NOT EXISTS deleted_by_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
