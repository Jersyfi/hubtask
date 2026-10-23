-- A picture of one's own (data-catalog.md, `account`): `account.avatar_media_id` names the media
-- object a person shows beside their name, and `AVATAR` is the usage such an object is staged for.
--
-- The reference is cleared, not refused, when the object goes (`ON DELETE SET NULL` of this column
-- alone): a replace-restore empties `media_object` before `account`, and an erasure removes the
-- person's uploads after anonymising the row - both must leave an account without a picture rather
-- than fail. The media reconciliation counts the reference, so a picture somebody shows is never
-- reclaimed underneath them.
--
-- Expand only: a nullable column without a default rewrites nothing, the constraint is widened,
-- and a process of the previous release neither writes nor reads either (rule 12).

-- +goose Up
ALTER TABLE account ADD COLUMN IF NOT EXISTS avatar_media_id uuid;
ALTER TABLE account DROP CONSTRAINT IF EXISTS account_avatar_media_fkey;
ALTER TABLE account ADD CONSTRAINT account_avatar_media_fkey
  FOREIGN KEY (tenant_id, avatar_media_id) REFERENCES media_object (tenant_id, id)
  ON DELETE SET NULL (avatar_media_id);

ALTER TABLE media_object DROP CONSTRAINT IF EXISTS media_object_usage_check;
ALTER TABLE media_object ADD CONSTRAINT media_object_usage_check
  CHECK (usage IN ('COVER', 'ATTACHMENT', 'IMPORT', 'EXPORT', 'AVATAR'));

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
