-- A desk's own logo (UC-SVC-05 check 9): `container.logo_media_id` names the picture the portal
-- shows for a collection serving as a service, and `DESK_LOGO` is the usage it is staged for.
--
-- Cleared, not refused, when the object goes (`ON DELETE SET NULL` of this column alone), for
-- migration 0199's reason: a replace-restore empties `media_object` before `container`, and an
-- erasure removes the uploader's objects - both must leave a desk drawn by its icon rather than
-- fail. The media reconciliation counts the reference.
--
-- Expand only (rule 12): a nullable column without a default rewrites nothing, the usage CHECK is
-- widened, and a process of the previous release neither writes nor reads either.

-- +goose Up
ALTER TABLE container ADD COLUMN IF NOT EXISTS logo_media_id uuid;
ALTER TABLE container DROP CONSTRAINT IF EXISTS container_logo_media_fkey;
ALTER TABLE container ADD CONSTRAINT container_logo_media_fkey
  FOREIGN KEY (tenant_id, logo_media_id) REFERENCES media_object (tenant_id, id)
  ON DELETE SET NULL (logo_media_id);

ALTER TABLE media_object DROP CONSTRAINT IF EXISTS media_object_usage_check;
ALTER TABLE media_object ADD CONSTRAINT media_object_usage_check
  CHECK (usage IN ('COVER', 'ATTACHMENT', 'IMPORT', 'EXPORT', 'AVATAR', 'DESK_LOGO'));

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
