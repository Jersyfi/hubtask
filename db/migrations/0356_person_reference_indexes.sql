-- The indexes behind the foreign keys an account's erasure follows (and, for container's logo, a
-- medium's delete): each SET NULL or CASCADE without one scanned the tenant's rows of its table
-- once per deleted account - realtime_update and page_version among them, a row per update and
-- per saved version - and ran the erasure past its statement timeout (known-traps.md, "A cascade
-- without an index"). Partial on the column being set, so the rows nobody references cost
-- nothing. CONCURRENTLY and without a transaction, so no write waits while they build
-- (versioning-release.md §4); the migrator rebuilds one an interrupted build left invalid.

-- +goose NO TRANSACTION

-- +goose Up

CREATE INDEX CONCURRENTLY IF NOT EXISTS realtime_update_author_id_ref_idx
  ON realtime_update (tenant_id, author_id) WHERE author_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_owner_id_ref_idx
  ON page (tenant_id, owner_id) WHERE owner_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_version_author_id_ref_idx
  ON page_version (tenant_id, author_id) WHERE author_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_comment_author_id_ref_idx
  ON page_comment (tenant_id, author_id) WHERE author_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS whiteboard_version_author_id_ref_idx
  ON whiteboard_version (tenant_id, author_id) WHERE author_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS whiteboard_comment_author_id_ref_idx
  ON whiteboard_comment (tenant_id, author_id) WHERE author_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS session_viewing_as_ref_idx
  ON session (tenant_id, viewing_as) WHERE viewing_as IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS cmdb_asset_assigned_to_ref_idx
  ON cmdb_asset (tenant_id, assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_restriction_granted_by_ref_idx
  ON page_restriction (tenant_id, granted_by) WHERE granted_by IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_restriction_account_id_ref_idx
  ON page_restriction (tenant_id, account_id) WHERE account_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_share_granted_by_ref_idx
  ON page_share (tenant_id, granted_by) WHERE granted_by IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_public_link_created_by_ref_idx
  ON page_public_link (tenant_id, created_by) WHERE created_by IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_article_published_by_ref_idx
  ON page_article (tenant_id, published_by) WHERE published_by IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_article_rating_account_id_ref_idx
  ON page_article_rating (tenant_id, account_id) WHERE account_id IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS portfolio_public_link_created_by_ref_idx
  ON portfolio_public_link (tenant_id, created_by) WHERE created_by IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS public_form_created_by_ref_idx
  ON public_form (tenant_id, created_by) WHERE created_by IS NOT NULL;
CREATE INDEX CONCURRENTLY IF NOT EXISTS container_logo_media_id_ref_idx
  ON container (tenant_id, logo_media_id) WHERE logo_media_id IS NOT NULL;

-- +goose Down

DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
