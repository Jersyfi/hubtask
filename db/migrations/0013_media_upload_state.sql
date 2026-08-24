-- The media object's upload life, and the cover's integrity (C-06).
--
-- `status` is the presigned flow written into the row: PENDING between staging and confirmation,
-- READY once the bytes were read back, judged and sealed. The default is PENDING - fail closed:
-- a row that somehow skipped the explicit value must not read as already judged. `file_name` is
-- the name the file arrived under, for the download; the data catalogue's media row already
-- names it ("file, name, checksum").
--
-- The cover gains what item_attachment has had since 0001: a tenant-scoped foreign key with
-- ON DELETE RESTRICT, so a media object under a cover cannot be removed out from under it and
-- the deletion ordering is impossible to get wrong. The consistency CHECK pins what the domain
-- promises - a colour cover carries a token and no image, an image cover the reverse, no cover
-- carries neither. No CHECK ties covers to TASK: which types carry COVER is the capability
-- matrix, which is data per tenant, and a schema constraint would harden a configurable rule.
--
-- Forward-only and safe for a rolling update (CLAUDE.md rule 12, ADR-0003): the columns arrive
-- with defaults, and both constraints are added NOT VALID - a catalogue change under the brief
-- lock - then validated in a second statement writes can live with. Every existing row is
-- coverless and the media table has never had a writer, so neither validation can fail.

-- +goose Up

ALTER TABLE media_object
  ADD COLUMN status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'READY')),
  ADD COLUMN file_name text;

ALTER TABLE work_item ADD CONSTRAINT work_item_cover_consistent CHECK (
  (cover_kind IS NULL AND cover_color_token IS NULL AND cover_media_id IS NULL)
  OR (cover_kind = 'COLOR' AND cover_color_token IS NOT NULL AND cover_media_id IS NULL)
  OR (cover_kind = 'IMAGE' AND cover_media_id IS NOT NULL AND cover_color_token IS NULL)
) NOT VALID;
ALTER TABLE work_item VALIDATE CONSTRAINT work_item_cover_consistent;

ALTER TABLE work_item ADD CONSTRAINT work_item_cover_media_fkey
  FOREIGN KEY (tenant_id, cover_media_id)
  REFERENCES media_object (tenant_id, id) ON DELETE RESTRICT
  NOT VALID;
ALTER TABLE work_item VALIDATE CONSTRAINT work_item_cover_media_fkey;
