-- A page shared with a service's portal as a space: everything under it reaches the same
-- requesters (PO-4, E-13; UC-DOC-13). A page behind a restriction, and everything under it, never
-- does; that is read at each request, not stored.
--
-- Expand only (rule 12): a column with a default nothing older reads; false keeps every existing
-- publication a single article.
-- +goose Up
ALTER TABLE page_article ADD COLUMN IF NOT EXISTS with_subpages boolean NOT NULL DEFAULT false;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
