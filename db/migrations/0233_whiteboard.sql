-- The whiteboard module's boards (M4): a hub's boards and where they point.
--
-- * `whiteboard` sits in a hub. Its `scene` is the projection the client saves beside the live
--   document (realtime, kind `whiteboard`): a JSON array of the board's objects in drawing order,
--   in the contract's shape (schema WhiteboardElement). The server never reads the CRDT.
-- * `whiteboard_reference` is where a board points - each card's entry and the `hubtask:` links in
--   its objects' words - kept by the trigger below from the scene in the writing transaction, with
--   the grammar of `entry_references_of` (migration 0213). It is what an entry's boards read. It
--   is not archived: a restore's write of the scene rebuilds it.
--
-- Expand only: new tables nothing older reads.
-- +goose Up
CREATE TABLE IF NOT EXISTS whiteboard (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id     uuid NOT NULL,
  title      text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  scene      jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(scene) = 'array' AND jsonb_array_length(scene) <= 5000),
  deleted_at timestamptz,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL,
  updated_by uuid NOT NULL,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT whiteboard_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS whiteboard_hub_idx ON whiteboard (tenant_id, hub_id, updated_at DESC);

ALTER TABLE whiteboard ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard;
CREATE POLICY tenant_isolation ON whiteboard
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON whiteboard TO hubtask_app;

CREATE TABLE IF NOT EXISTS whiteboard_reference (
  tenant_id     uuid NOT NULL,
  whiteboard_id uuid NOT NULL,
  target_kind   text NOT NULL CHECK (target_kind IN
                  ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')),
  target_id     uuid NOT NULL,
  anchor        text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  strength      text NOT NULL CHECK (strength IN ('MENTION', 'CARD', 'EMBED')),
  PRIMARY KEY (tenant_id, whiteboard_id, target_kind, target_id, anchor),
  CONSTRAINT whiteboard_reference_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS whiteboard_reference_target_idx
  ON whiteboard_reference (tenant_id, target_kind, target_id);

ALTER TABLE whiteboard_reference ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_reference FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard_reference;
CREATE POLICY tenant_isolation ON whiteboard_reference
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON whiteboard_reference TO hubtask_app;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION whiteboard_reference_of_scene() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM whiteboard_reference WHERE tenant_id = NEW.tenant_id AND whiteboard_id = NEW.id;
  -- A trashed board points nowhere until it is restored.
  IF NEW.deleted_at IS NULL THEN
    INSERT INTO whiteboard_reference (tenant_id, whiteboard_id, target_kind, target_id, anchor, strength)
    SELECT NEW.tenant_id, NEW.id, found.target_kind, found.target_id, found.anchor,
           (array_agg(found.strength ORDER BY found.rank DESC))[1]
    FROM (
      SELECT e #>> '{target,kind}' AS target_kind, (e #>> '{target,id}')::uuid AS target_id,
             '' AS anchor, 'CARD' AS strength, 1 AS rank
        FROM jsonb_array_elements(NEW.scene) AS e
       WHERE e ->> 'kind' = 'CARD' AND e #>> '{target,id}' IS NOT NULL
      UNION ALL
      SELECT r.target_kind, r.target_id, r.anchor, r.strength,
             CASE r.strength WHEN 'EMBED' THEN 2 WHEN 'CARD' THEN 1 ELSE 0 END
        FROM jsonb_array_elements(NEW.scene) AS e, entry_references_of(e ->> 'text') AS r
       WHERE position('hubtask:' IN coalesce(e ->> 'text', '')) > 0
    ) AS found
    WHERE NOT (found.target_kind = 'WHITEBOARD' AND found.target_id = NEW.id)
    GROUP BY found.target_kind, found.target_id, found.anchor;
  END IF;
  RETURN NULL;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS whiteboard_references_insert ON whiteboard;
CREATE TRIGGER whiteboard_references_insert AFTER INSERT ON whiteboard
  FOR EACH ROW EXECUTE FUNCTION whiteboard_reference_of_scene();
DROP TRIGGER IF EXISTS whiteboard_references_update ON whiteboard;
CREATE TRIGGER whiteboard_references_update AFTER UPDATE OF scene, deleted_at ON whiteboard
  FOR EACH ROW EXECUTE FUNCTION whiteboard_reference_of_scene();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
