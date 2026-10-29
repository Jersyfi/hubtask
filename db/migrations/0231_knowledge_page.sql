-- The knowledge module's pages (M3): a hub's space, its page tree, each page's versions, the
-- comments beside it and where its text points.
--
-- * `page` sits in a hub (its space) and under another page or at the top. Its `body` is the
--   Markdown projection the editor saves beside the live document (realtime, kind `page`); the
--   server never reads the CRDT. `search_document` is kept by a trigger from the title and body in
--   the page's content language, as an entry's is (migration 0019), so every door that writes a
--   page keeps it.
-- * `page_version` is a page's history: one row per editing session, a named or a restored
--   version its own row (UC-DOC-06).
-- * `page_comment` is a thread on the page or inline on a block (`anchor`) with the words it was
--   about (`quote`).
-- * `page_reference` is where a page's body points, kept by the trigger below from the body in
--   the writing transaction, with the grammar of `entry_references_of` (migration 0213). It is what
--   a page's and an entry's backlinks read for pages. It is not archived: a restore's write of the
--   body rebuilds it.
--
-- Expand only: new tables nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS page (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id           uuid NOT NULL,
  parent_id        uuid,
  title            text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  body             text NOT NULL DEFAULT '' CHECK (octet_length(body) <= 1048576),
  icon             text NOT NULL DEFAULT '' CHECK (char_length(icon) <= 16),
  order_key        text NOT NULL,
  template         boolean NOT NULL DEFAULT false,
  content_language text,
  owner_id         uuid,
  verified_until   timestamptz,
  verified_at      timestamptz,
  verified_by      uuid,
  archived_at      timestamptz,
  deleted_at       timestamptz,
  created_by       uuid NOT NULL,
  created_at       timestamptz NOT NULL,
  updated_by       uuid NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  search_document  tsvector,
  UNIQUE (tenant_id, id),
  CHECK (parent_id IS NULL OR parent_id <> id),
  CONSTRAINT page_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_parent_fkey FOREIGN KEY (tenant_id, parent_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_owner_fkey FOREIGN KEY (tenant_id, owner_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (owner_id)
);
-- The tree a space reads: its pages in order under each parent.
CREATE INDEX IF NOT EXISTS page_tree_idx ON page (tenant_id, hub_id, parent_id, order_key);
CREATE INDEX IF NOT EXISTS page_search_idx ON page USING gin (search_document);
-- The review list: the pages whose verification runs out.
CREATE INDEX IF NOT EXISTS page_verified_until_idx ON page (tenant_id, verified_until)
  WHERE verified_until IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE page ENABLE ROW LEVEL SECURITY;
ALTER TABLE page FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page;
CREATE POLICY tenant_isolation ON page
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page TO hubtask_app;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION page_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.body);
  RETURN NEW;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS page_search_document ON page;
CREATE TRIGGER page_search_document
  BEFORE INSERT OR UPDATE OF title, body, content_language ON page
  FOR EACH ROW EXECUTE FUNCTION page_search_document();

CREATE TABLE IF NOT EXISTS page_version (
  tenant_id  uuid NOT NULL,
  page_id    uuid NOT NULL,
  seq        integer NOT NULL CHECK (seq > 0),
  title      text NOT NULL,
  body       text NOT NULL,
  author_id  uuid,
  name       text NOT NULL DEFAULT '' CHECK (char_length(name) <= 120),
  restored   integer NOT NULL DEFAULT 0 CHECK (restored >= 0),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, seq),
  CONSTRAINT page_version_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_version_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);

ALTER TABLE page_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_version FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_version;
CREATE POLICY tenant_isolation ON page_version
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_version TO hubtask_app;

CREATE TABLE IF NOT EXISTS page_comment (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id     uuid NOT NULL,
  parent_id   uuid,
  author_id   uuid,
  body        text NOT NULL CHECK (octet_length(body) <= 65536),
  anchor      text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  quote       text NOT NULL DEFAULT '' CHECK (char_length(quote) <= 500),
  resolved_at timestamptz,
  resolved_by uuid,
  created_at  timestamptz NOT NULL,
  edited_at   timestamptz,
  deleted_at  timestamptz,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT page_comment_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_comment_parent_fkey FOREIGN KEY (tenant_id, parent_id)
    REFERENCES page_comment (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_comment_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
CREATE INDEX IF NOT EXISTS page_comment_page_idx ON page_comment (tenant_id, page_id, created_at);

ALTER TABLE page_comment ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_comment FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_comment;
CREATE POLICY tenant_isolation ON page_comment
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON page_comment TO hubtask_app;

CREATE TABLE IF NOT EXISTS page_reference (
  tenant_id   uuid NOT NULL,
  page_id     uuid NOT NULL,
  target_kind text NOT NULL CHECK (target_kind IN
                ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')),
  target_id   uuid NOT NULL,
  anchor      text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  strength    text NOT NULL CHECK (strength IN ('MENTION', 'CARD', 'EMBED')),
  PRIMARY KEY (tenant_id, page_id, target_kind, target_id, anchor),
  CONSTRAINT page_reference_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS page_reference_target_idx ON page_reference (tenant_id, target_kind, target_id);

ALTER TABLE page_reference ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_reference FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON page_reference;
CREATE POLICY tenant_isolation ON page_reference
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON page_reference TO hubtask_app;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION page_reference_of_body() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM page_reference WHERE tenant_id = NEW.tenant_id AND page_id = NEW.id;
  -- A trashed page points nowhere until it is restored.
  IF NEW.deleted_at IS NULL AND position('hubtask:' IN NEW.body) > 0 THEN
    INSERT INTO page_reference (tenant_id, page_id, target_kind, target_id, anchor, strength)
    SELECT NEW.tenant_id, NEW.id, r.target_kind, r.target_id, r.anchor, r.strength
      FROM entry_references_of(NEW.body) AS r
     WHERE NOT (r.target_kind = 'PAGE' AND r.target_id = NEW.id);
  END IF;
  RETURN NULL;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS page_references_insert ON page;
CREATE TRIGGER page_references_insert AFTER INSERT ON page
  FOR EACH ROW EXECUTE FUNCTION page_reference_of_body();
DROP TRIGGER IF EXISTS page_references_update ON page;
CREATE TRIGGER page_references_update AFTER UPDATE OF body, deleted_at ON page
  FOR EACH ROW EXECUTE FUNCTION page_reference_of_body();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
