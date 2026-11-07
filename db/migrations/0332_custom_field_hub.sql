-- A custom field defined at a hub (configuration model §1, §2; UC-WRK-12 check 9): the entries of
-- the hub's collections carry it, a collection's own field under the key wins over it and it wins
-- over the workspace's, and a copy of a shared field records the definition it was made from.
--
-- The hub is stored where a collection is, in `collection_id`: that column already references
-- `container`, which holds hubs and collections alike, and its unique index already keeps a key
-- free once per container - so a hub needs no column of its own, and the hub goes with its fields
-- through the cascade that takes a collection's. Whether the container is a hub is read from the
-- container. `copied_from` is an identifier, not a reference: the copy keeps saying where it came
-- from after the source is gone.
--
-- Expand only (rule 12): one nullable column. An older binary matches a definition's container
-- against an entry's collection alone, so it shows a hub's fields nowhere, and writes none.

-- A hub's definition records its change under the hub (custom_field_figure_touch, 0299), so a
-- collection's number of changes counts its hub's as well: a kept figure of the collection was
-- counted with the fields in force then, a hub's among them. Replaced in place; an older binary
-- calls the same function and only ever sees a kept count expire sooner.
--
-- +goose Up
ALTER TABLE custom_field_definition ADD COLUMN IF NOT EXISTS copied_from uuid;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION collection_figure_changes(collection uuid) RETURNS bigint
LANGUAGE sql STABLE SET search_path = public, pg_temp AS $$
  WITH scopes AS (
    SELECT collection AS id
    UNION ALL SELECT current_tenant_id()
    UNION ALL SELECT c.parent_id FROM container c WHERE c.id = collection AND c.parent_id IS NOT NULL
  )
  SELECT coalesce((SELECT sum(s.folded) FROM collection_figure_stamp s
                    WHERE s.tenant_id = current_tenant_id()
                      AND s.collection_id IN (SELECT id FROM scopes)), 0)::bigint
       + (SELECT count(*) FROM collection_figure_touch t
           WHERE t.tenant_id = current_tenant_id()
             AND t.collection_id IN (SELECT id FROM scopes))
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
