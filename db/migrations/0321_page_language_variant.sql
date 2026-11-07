-- A page written in another language for a service's portal (UC-DOC-13 check 4):
-- `page.language_source_id` names the page this one is a language variant of, and
-- `page.language_variant` the BCP 47 tag it is that variant under. A portal reads a published
-- page's variant in the requester's language in its place.
--
-- * One variant per tag and source, whatever the tag's case.
-- * Cleared, not refused, when the source goes (`ON DELETE SET NULL` of the link alone): an emptied
--   trash leaves the variant a page of its own, and the reader ignores a tag without a source.
-- * Deferred to the commit, as `page_parent_fkey`: a restore writes pages in identifier order.
--
-- Expand only (rule 12): two nullable columns without a default rewrite nothing, the index covers
-- only linked pages, and a process of the previous release neither writes nor reads them.

-- +goose Up
ALTER TABLE page ADD COLUMN IF NOT EXISTS language_source_id uuid;
ALTER TABLE page ADD COLUMN IF NOT EXISTS language_variant text;
ALTER TABLE page DROP CONSTRAINT IF EXISTS page_language_source_fkey;
ALTER TABLE page ADD CONSTRAINT page_language_source_fkey
  FOREIGN KEY (tenant_id, language_source_id) REFERENCES page (tenant_id, id)
  ON DELETE SET NULL (language_source_id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE page DROP CONSTRAINT IF EXISTS page_language_variant_check;
ALTER TABLE page ADD CONSTRAINT page_language_variant_check
  CHECK (language_variant IS NULL OR char_length(language_variant) BETWEEN 1 AND 35);
CREATE UNIQUE INDEX IF NOT EXISTS page_language_variant_idx
  ON page (tenant_id, language_source_id, lower(language_variant))
  WHERE language_source_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
