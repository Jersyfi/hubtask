-- Links and references (F-3, P-19, UC-PLN-01). Three tables, all new, so expand only
-- (versioning-release.md §4):
--
-- * `link_type` holds a workspace's own kinds of link. The six Hubtask ships (blocks, depends_on,
--   duplicates, runs_on, affects, implements) are declared in the domain and have no row, as the
--   system entry types have none (0209); a workspace's key never takes one of theirs.
-- * `entry_link` is one link of a type from one entry to another. It is drawn and removed, never
--   changed. A workspace's own type is held by a foreign key on the generated `own_type`, so a type
--   in use cannot be deleted, only archived; a shipped type needs no row to point at.
-- * `entry_reference` is where a text points (a mention, a card, a live view): the projection of
--   the `hubtask:<kind>/<id>` addresses an entry's notes and its comments hold, kept by the
--   triggers below in the writing transaction, so every door that writes a note or a comment keeps
--   it - REST, MCP, sync push, rules, import, a restore. It is what the backlinks read. It is not
--   archived: a restore's own write of the notes and comments rebuilds it. No text written before
--   this migration holds an address of that grammar, so there is nothing to backfill.

-- +goose Up
CREATE TABLE IF NOT EXISTS link_type (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'
                 AND key NOT IN ('blocks', 'depends_on', 'duplicates', 'runs_on', 'affects', 'implements')),
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  -- Empty only on a symmetric type, which reads the same from both ends.
  inverse_name text NOT NULL CHECK (char_length(inverse_name) <= 120),
  direction    text NOT NULL CHECK (direction IN ('DIRECTED', 'SYMMETRIC')),
  cardinality  text NOT NULL CHECK (cardinality IN ('MANY', 'ONE')),
  cycles       text NOT NULL CHECK (cycles IN ('FORBIDDEN', 'ALLOWED')),
  waits        text NOT NULL CHECK (waits IN ('NONE', 'SOURCE', 'TARGET')),
  position     integer NOT NULL DEFAULT 0,
  archived_at  timestamptz,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT link_type_key_uq UNIQUE (tenant_id, key),
  CHECK (direction = 'DIRECTED' OR (waits = 'NONE' AND cardinality = 'MANY')),
  CHECK (direction = 'SYMMETRIC' OR inverse_name <> ''),
  CHECK (waits = 'NONE' OR cycles = 'FORBIDDEN')
);

ALTER TABLE link_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE link_type FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON link_type
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON link_type TO hubtask_app;

CREATE TABLE IF NOT EXISTS entry_link (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  link_type   text NOT NULL CHECK (link_type ~ '^[a-z][a-z0-9_]{0,49}$'),
  own_type    text GENERATED ALWAYS AS (
                CASE WHEN link_type IN ('blocks', 'depends_on', 'duplicates', 'runs_on', 'affects', 'implements')
                     THEN NULL ELSE link_type END) STORED,
  source_id   uuid NOT NULL,
  target_id   uuid NOT NULL,
  created_by  uuid,
  created_at  timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CONSTRAINT entry_link_uq UNIQUE (tenant_id, link_type, source_id, target_id),
  CHECK (source_id <> target_id),
  CONSTRAINT entry_link_source_fkey
    FOREIGN KEY (tenant_id, source_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT entry_link_target_fkey
    FOREIGN KEY (tenant_id, target_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT entry_link_type_fkey
    FOREIGN KEY (tenant_id, own_type) REFERENCES link_type (tenant_id, key)
);
-- An entry's links in both directions: its own list and its backlinks.
CREATE INDEX IF NOT EXISTS entry_link_source_idx ON entry_link (tenant_id, source_id, link_type);
CREATE INDEX IF NOT EXISTS entry_link_target_idx ON entry_link (tenant_id, target_id, link_type);
CREATE INDEX IF NOT EXISTS entry_link_own_type_idx ON entry_link (tenant_id, own_type) WHERE own_type IS NOT NULL;

ALTER TABLE entry_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_link FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entry_link
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON entry_link TO hubtask_app;

CREATE TABLE IF NOT EXISTS entry_reference (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- The text that points: an entry's notes or one of its comments.
  source_kind text NOT NULL CHECK (source_kind IN ('ENTRY', 'COMMENT')),
  source_id   uuid NOT NULL,
  -- The entry the text belongs to: what a reader's permission to see the source is judged on.
  entry_id    uuid NOT NULL,
  target_kind text NOT NULL CHECK (target_kind IN
                ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')),
  target_id   uuid NOT NULL,
  anchor      text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  strength    text NOT NULL CHECK (strength IN ('MENTION', 'CARD', 'EMBED')),
  PRIMARY KEY (tenant_id, source_kind, source_id, target_kind, target_id, anchor),
  CONSTRAINT entry_reference_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS entry_reference_target_idx ON entry_reference (tenant_id, target_kind, target_id);

ALTER TABLE entry_reference ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_reference FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entry_reference
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON entry_reference TO hubtask_app;

-- The addresses a Markdown text holds, once per object and anchor, the strongest drawing winning:
-- the grammar of reference.Extract, which the integration test holds this function to.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entry_references_of(body text)
RETURNS TABLE (target_kind text, target_id uuid, anchor text, strength text)
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT found.kind, found.id, found.anchor,
         (array_agg(found.strength ORDER BY found.rank DESC))[1]
  FROM (
    SELECT upper(m[2]) AS kind, lower(m[3])::uuid AS id, coalesce(m[4], '') AS anchor,
           CASE WHEN m[1] <> '!' THEN 'MENTION'
                WHEN upper(m[2]) IN ('VIEW', 'DASHBOARD', 'WHITEBOARD') THEN 'EMBED'
                ELSE 'CARD' END AS strength,
           CASE WHEN m[1] <> '!' THEN 0
                WHEN upper(m[2]) IN ('VIEW', 'DASHBOARD', 'WHITEBOARD') THEN 2
                ELSE 1 END AS rank
    FROM regexp_matches(
      coalesce(body, ''),
      '(!?)\[[^\]]*\]\(hubtask:([a-z]+)/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:#([A-Za-z0-9_-]{1,64}))?\)',
      'g') AS m
    WHERE upper(m[2]) IN ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')
  ) AS found
  GROUP BY found.kind, found.id, found.anchor
  LIMIT 200
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entry_reference_of_notes() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM entry_reference
   WHERE tenant_id = NEW.tenant_id AND source_kind = 'ENTRY' AND source_id = NEW.id;
  IF NEW.notes IS NOT NULL AND position('hubtask:' IN NEW.notes) > 0 THEN
    INSERT INTO entry_reference (tenant_id, source_kind, source_id, entry_id, target_kind, target_id, anchor, strength)
    SELECT NEW.tenant_id, 'ENTRY', NEW.id, NEW.id, r.target_kind, r.target_id, r.anchor, r.strength
      FROM entry_references_of(NEW.notes) AS r
     WHERE NOT (r.target_kind = 'ENTRY' AND r.target_id = NEW.id);
  END IF;
  RETURN NULL;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION entry_reference_of_comment() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM entry_reference
   WHERE tenant_id = NEW.tenant_id AND source_kind = 'COMMENT' AND source_id = NEW.id;
  -- A deleted comment points nowhere; its text is gone or held only for a legal hold.
  IF NEW.deleted_at IS NULL AND position('hubtask:' IN NEW.body) > 0 THEN
    INSERT INTO entry_reference (tenant_id, source_kind, source_id, entry_id, target_kind, target_id, anchor, strength)
    SELECT NEW.tenant_id, 'COMMENT', NEW.id, NEW.item_id, r.target_kind, r.target_id, r.anchor, r.strength
      FROM entry_references_of(NEW.body) AS r;
  END IF;
  RETURN NULL;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS work_item_references_insert ON work_item;
CREATE TRIGGER work_item_references_insert AFTER INSERT ON work_item
  FOR EACH ROW WHEN (NEW.notes IS NOT NULL) EXECUTE FUNCTION entry_reference_of_notes();
DROP TRIGGER IF EXISTS work_item_references_update ON work_item;
CREATE TRIGGER work_item_references_update AFTER UPDATE OF notes ON work_item
  FOR EACH ROW WHEN (NEW.notes IS DISTINCT FROM OLD.notes) EXECUTE FUNCTION entry_reference_of_notes();
DROP TRIGGER IF EXISTS comment_references_insert ON comment;
CREATE TRIGGER comment_references_insert AFTER INSERT ON comment
  FOR EACH ROW EXECUTE FUNCTION entry_reference_of_comment();
DROP TRIGGER IF EXISTS comment_references_update ON comment;
CREATE TRIGGER comment_references_update AFTER UPDATE OF body, deleted_at ON comment
  FOR EACH ROW WHEN (NEW.body IS DISTINCT FROM OLD.body OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at)
  EXECUTE FUNCTION entry_reference_of_comment();

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
