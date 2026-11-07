-- A page published as an article of a service's portal, and what requesters said of it (M3:
-- knowledge-base publishing for the portal; UC-DOC-13).
--
-- * `page_article` publishes one page to one service - a collection whose portal a requester
--   reaches with REQUEST. The audience is the service: a page reaches the requesters of each
--   collection it is published to, and nobody else. `views` counts its openings in the portal.
-- * `page_article_rating` is one requester's "was this helpful?", one per person and service.
--
-- Expand only (rule 12): new tables nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS page_article (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id       uuid NOT NULL,
  collection_id uuid NOT NULL,
  published_by  uuid,
  published_at  timestamptz NOT NULL,
  views         bigint NOT NULL DEFAULT 0 CHECK (views >= 0),
  PRIMARY KEY (tenant_id, page_id, collection_id),
  CONSTRAINT page_article_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_article_collection_fkey FOREIGN KEY (tenant_id, collection_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_article_published_by_fkey FOREIGN KEY (tenant_id, published_by)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (published_by)
);
-- The articles of one service, the portal's question.
CREATE INDEX IF NOT EXISTS page_article_collection_idx ON page_article (tenant_id, collection_id);

CREATE TABLE IF NOT EXISTS page_article_rating (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id       uuid NOT NULL,
  collection_id uuid NOT NULL,
  account_id    uuid NOT NULL,
  helpful       boolean NOT NULL,
  rated_at      timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, collection_id, account_id),
  CONSTRAINT page_article_rating_article_fkey FOREIGN KEY (tenant_id, page_id, collection_id)
    REFERENCES page_article (tenant_id, page_id, collection_id) ON DELETE CASCADE,
  CONSTRAINT page_article_rating_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE page_article ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_article FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_article;
CREATE POLICY tenant_isolation ON page_article
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_article TO hubtask_app;

ALTER TABLE page_article_rating ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_article_rating FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_article_rating;
CREATE POLICY tenant_isolation ON page_article_rating
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_article_rating TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
