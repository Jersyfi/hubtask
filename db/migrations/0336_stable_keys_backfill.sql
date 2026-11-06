-- The handles and keys 0293 gives every new hub, saved view and automation rule, given to the rows
-- that existed before it: hubs after the collections, oldest first, one row per UPDATE for the
-- reason 0204 gives (a single UPDATE would not see the keys it has just given).
--
-- Outside a transaction and committed every 500 rows, so that no lock outlives its batch: inside
-- 0293 the walk held that file's exclusive locks on saved_view and automation_rule until the last
-- row. Each walk moves forward by (created_at, id) and records that position in migration_cursor
-- with each batch (versioning-release.md §4), so a run killed between batches resumes after the
-- last batch it committed.

-- +goose NO TRANSACTION

-- +goose Up
CREATE TABLE IF NOT EXISTS migration_cursor (
  name       text PRIMARY KEY CHECK (name ~ '^[0-9]{4}_[a-z0-9_]{1,80}$'),
  position   text NOT NULL CHECK (char_length(position) <= 200),
  updated_at timestamptz NOT NULL
);
REVOKE ALL ON migration_cursor FROM hubtask_app;

-- +goose StatementBegin
DO $backfill$
DECLARE
  walk     text;
  saved    text;
  batch    uuid[];
  one      uuid;
  after_ts timestamptz;
  last_id  uuid;
BEGIN
  FOREACH walk IN ARRAY ARRAY['0336_stable_keys_hubs', '0336_stable_keys_views', '0336_stable_keys_rules'] LOOP
    SELECT position INTO saved FROM migration_cursor WHERE name = walk;
    after_ts := coalesce(split_part(saved, '|', 1)::timestamptz, '-infinity');
    last_id := coalesce(split_part(saved, '|', 2)::uuid, '00000000-0000-0000-0000-000000000000');
    LOOP
      IF walk = '0336_stable_keys_hubs' THEN
        SELECT array_agg(id ORDER BY created_at, id), max(created_at) INTO batch, after_ts
          FROM (SELECT id, created_at FROM container
                WHERE type = 'HUB' AND handle IS NULL AND (created_at, id) > (after_ts, last_id)
                ORDER BY created_at, id LIMIT 500) AS page;
      ELSIF walk = '0336_stable_keys_views' THEN
        SELECT array_agg(id ORDER BY created_at, id), max(created_at) INTO batch, after_ts
          FROM (SELECT id, created_at FROM saved_view
                WHERE key IS NULL AND (created_at, id) > (after_ts, last_id)
                ORDER BY created_at, id LIMIT 500) AS page;
      ELSE
        SELECT array_agg(id ORDER BY created_at, id), max(created_at) INTO batch, after_ts
          FROM (SELECT id, created_at FROM automation_rule
                WHERE key IS NULL AND (created_at, id) > (after_ts, last_id)
                ORDER BY created_at, id LIMIT 500) AS page;
      END IF;
      EXIT WHEN batch IS NULL;
      last_id := batch[array_upper(batch, 1)];

      FOREACH one IN ARRAY batch LOOP
        IF walk = '0336_stable_keys_hubs' THEN
          UPDATE container SET handle = container_free_handle(tenant_id, container_handle_base(name), id)
          WHERE id = one AND handle IS NULL;
        ELSIF walk = '0336_stable_keys_views' THEN
          -- Setting the key to NULL draws it through 0293's trigger.
          UPDATE saved_view SET key = NULL WHERE id = one AND key IS NULL;
        ELSE
          UPDATE automation_rule SET key = NULL WHERE id = one AND key IS NULL;
        END IF;
      END LOOP;

      INSERT INTO migration_cursor (name, position, updated_at)
      VALUES (walk, after_ts::text || '|' || last_id::text, clock_timestamp())
      ON CONFLICT (name) DO UPDATE SET position = EXCLUDED.position, updated_at = EXCLUDED.updated_at;
      COMMIT;
    END LOOP;
    DELETE FROM migration_cursor WHERE name = walk;
    COMMIT;
  END LOOP;
END $backfill$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
