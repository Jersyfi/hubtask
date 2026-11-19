-- Posts (work-surface concept E-15, B-1…B-6): a post is a page with a date. The page keeps its
-- words, cover, icon and labels; `post` adds when it goes out, for whom, whether it stays on top,
-- whether it is discussed and whether its readers confirm it. Who has seen it and who confirmed it
-- are personal data: kept with the post, gone with the person, never in the trail or a log
-- (rule 10, data-retention.md §3 under PAGE). The discussion is apart from the page's comments
-- (B-4): replies one level deep with a few fixed reactions.
--
-- Expand only (rule 12): new tables nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS post (
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id        uuid NOT NULL,
  hub_id         uuid NOT NULL,
  template       text CHECK (template IN ('ANNOUNCEMENT', 'RELEASE_NOTE', 'MONTHLY_REVIEW', 'WELCOME')),
  publish_at     timestamptz,
  published_at   timestamptz,
  featured_until timestamptz,
  audience       text NOT NULL DEFAULT 'HUB' CHECK (audience IN ('HUB', 'GROUPS')),
  group_ids      uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(group_ids) <= 20),
  discussion     boolean NOT NULL DEFAULT true,
  notify         boolean NOT NULL DEFAULT true,
  confirm_by     timestamptz,
  PRIMARY KEY (tenant_id, page_id),
  CONSTRAINT post_audience_groups CHECK ((audience = 'GROUPS') = (cardinality(group_ids) > 0)),
  CONSTRAINT post_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
-- A hub's posts, newest first; the scheduler's due posts across the installation.
CREATE INDEX IF NOT EXISTS post_hub_idx ON post (tenant_id, hub_id, publish_at DESC);
CREATE INDEX IF NOT EXISTS post_due_idx ON post (publish_at) WHERE published_at IS NULL AND publish_at IS NOT NULL;

-- Who has seen a post: counted for everybody ("seen by 38"), never listed by name.
CREATE TABLE IF NOT EXISTS post_read (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id    uuid NOT NULL,
  account_id uuid NOT NULL,
  read_at    timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, account_id),
  CONSTRAINT post_read_post_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES post (tenant_id, page_id) ON DELETE CASCADE,
  CONSTRAINT post_read_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS post_read_account_idx ON post_read (tenant_id, account_id);

-- Who confirmed reading a post that asks for it, shown to its writers only ("21 of 45").
CREATE TABLE IF NOT EXISTS post_confirmation (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id      uuid NOT NULL,
  account_id   uuid NOT NULL,
  confirmed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, account_id),
  CONSTRAINT post_confirmation_post_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES post (tenant_id, page_id) ON DELETE CASCADE,
  CONSTRAINT post_confirmation_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS post_confirmation_account_idx ON post_confirmation (tenant_id, account_id);

-- The discussion under a post: replies, one level deep.
CREATE TABLE IF NOT EXISTS post_reply (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id    uuid NOT NULL,
  parent_id  uuid,
  author_id  uuid,
  body       text NOT NULL CHECK (octet_length(body) <= 65536),
  created_at timestamptz NOT NULL,
  edited_at  timestamptz,
  deleted_at timestamptz,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT post_reply_post_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES post (tenant_id, page_id) ON DELETE CASCADE,
  CONSTRAINT post_reply_parent_fkey FOREIGN KEY (tenant_id, parent_id)
    REFERENCES post_reply (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT post_reply_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
CREATE INDEX IF NOT EXISTS post_reply_page_idx ON post_reply (tenant_id, page_id, created_at);
CREATE INDEX IF NOT EXISTS post_reply_author_idx ON post_reply (tenant_id, author_id);

-- A reaction to a reply: one of a few, once per person and kind.
CREATE TABLE IF NOT EXISTS post_reaction (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  reply_id   uuid NOT NULL,
  account_id uuid NOT NULL,
  reaction   text NOT NULL CHECK (reaction IN ('THANKS', 'AGREE', 'CELEBRATE', 'IDEA')),
  PRIMARY KEY (tenant_id, reply_id, account_id, reaction),
  CONSTRAINT post_reaction_reply_fkey FOREIGN KEY (tenant_id, reply_id)
    REFERENCES post_reply (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT post_reaction_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS post_reaction_account_idx ON post_reaction (tenant_id, account_id);

-- +goose StatementBegin
DO $rls$
DECLARE each text;
BEGIN
  FOREACH each IN ARRAY ARRAY['post', 'post_read', 'post_confirmation', 'post_reply', 'post_reaction'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', each);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', each);
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', each);
    EXECUTE format('CREATE POLICY tenant_isolation ON %I USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id())', each);
  END LOOP;
END $rls$;
-- +goose StatementEnd
GRANT SELECT, INSERT, UPDATE, DELETE ON post, post_reply TO hubtask_app;
GRANT SELECT, INSERT, DELETE ON post_read, post_confirmation, post_reaction TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
