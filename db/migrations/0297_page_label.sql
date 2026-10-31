-- The labels a page carries (UC-DOC-01 check 9): a space's own vocabulary, written on its pages and
-- counted as the facet a search narrows by. A label is the page's words, so it leaves with the page
-- and is never written into the trail in clear (rule 10).
--
-- Expand only (rule 12): a new table nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS page_label (
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id   uuid NOT NULL,
  label     text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 40 AND label = lower(label)),
  PRIMARY KEY (tenant_id, page_id, label),
  CONSTRAINT page_label_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
-- A search narrowed to a label.
CREATE INDEX IF NOT EXISTS page_label_label_idx ON page_label (tenant_id, label);

ALTER TABLE page_label ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_label FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_label;
CREATE POLICY tenant_isolation ON page_label
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON page_label TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
