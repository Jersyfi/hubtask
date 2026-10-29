-- A configuration item as a reference kind (P-19, M6): `hubtask:ci/<id>` in an entry's notes or
-- comments, a page's body or a whiteboard's words and cards is kept as the text's reference, so a
-- CI lists what points at it (ListCiBacklinks) and a reference resolves to it.
--
-- Expand only (rule 12): the three reference tables' target kind CHECKs widen by `CI` - added NOT
-- VALID and validated after, so the old rows are read without a long lock - and the address
-- grammar `entry_references_of` learns the kind. Older code never writes `CI`; a CI address it
-- keeps in a text is found when the text is next written.

-- +goose Up
ALTER TABLE entry_reference DROP CONSTRAINT IF EXISTS entry_reference_target_kind_check;
ALTER TABLE entry_reference ADD CONSTRAINT entry_reference_target_kind_check CHECK (target_kind IN
  ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT', 'CI')) NOT VALID;
ALTER TABLE entry_reference VALIDATE CONSTRAINT entry_reference_target_kind_check;

ALTER TABLE page_reference DROP CONSTRAINT IF EXISTS page_reference_target_kind_check;
ALTER TABLE page_reference ADD CONSTRAINT page_reference_target_kind_check CHECK (target_kind IN
  ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT', 'CI')) NOT VALID;
ALTER TABLE page_reference VALIDATE CONSTRAINT page_reference_target_kind_check;

ALTER TABLE whiteboard_reference DROP CONSTRAINT IF EXISTS whiteboard_reference_target_kind_check;
ALTER TABLE whiteboard_reference ADD CONSTRAINT whiteboard_reference_target_kind_check CHECK (target_kind IN
  ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT', 'CI')) NOT VALID;
ALTER TABLE whiteboard_reference VALIDATE CONSTRAINT whiteboard_reference_target_kind_check;

-- The grammar of reference.Extract with the kind `ci`; otherwise as migration 0213 wrote it.
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
    WHERE upper(m[2]) IN ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT', 'CI')
  ) AS found
  GROUP BY found.kind, found.id, found.anchor
  LIMIT 200
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
