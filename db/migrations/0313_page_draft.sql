-- A space with drafts (UC-DOC-15): what waits on a page to be published. A page never published
-- carries only the mark (its words are on the page, kept to its writer by a restriction); a
-- published page's draft holds the title and body its writers changed since. A draft is the
-- page's words, so it leaves with the page and is never written into the trail in clear (rule 10).
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS page_draft (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id     uuid NOT NULL,
  unpublished boolean NOT NULL DEFAULT false,
  title       text CHECK (title IS NULL OR char_length(title) BETWEEN 1 AND 200),
  body        text CHECK (body IS NULL OR octet_length(body) <= 1048576),
  updated_by  uuid NOT NULL,
  updated_at  timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id),
  CHECK (unpublished OR (title IS NOT NULL AND body IS NOT NULL)),
  CONSTRAINT page_draft_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);

ALTER TABLE page_draft ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_draft FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_draft;
CREATE POLICY tenant_isolation ON page_draft
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_draft TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
