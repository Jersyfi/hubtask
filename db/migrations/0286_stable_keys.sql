-- Stable keys for the blocks the configuration document names by key (accepted.md D, 2026-10-30;
-- config-document.md §1): a hub's handle beside a collection's, and a key for a saved view and an
-- automation rule, so that views and rules become sections of the workspace's and a hub's
-- document and a document names a hub or a collection by a word that is the same in a second
-- workspace.
--
-- A hub's handle is drawn by the same trigger a collection's is (migration 0204), in the same
-- namespace: one word names one place of the workspace. Mail still delivers to a collection only
-- (MailIntake.sql asks `type = 'COLLECTION'`). Existing collections keep their handles; hubs are
-- given theirs after them, so a clash numbers the hub, never a collection a mail address names.
--
-- A view's and a rule's key is lower case, digits and underscores, starting with a letter, at
-- most 63 (config.ValidKey). It is unique where the block lives - the workspace, one hub, one
-- collection, one account - so a hub's document applies to a second hub unchanged. A writer that
-- names none is given one from the name by the database, as a handle is; a writer that names one
-- already taken is refused by the unique index rather than renamed. A rename keeps the key.
--
-- Expand only (rule 12): two nullable columns the previous release neither reads nor writes, the
-- trigger filling them for its inserts too, and a hub's handle the previous release answers as
-- it answers a collection's.

-- +goose Up
ALTER TABLE saved_view ADD COLUMN IF NOT EXISTS key text
  CHECK (key IS NULL OR key ~ '^[a-z][a-z0-9_]{0,62}$');
ALTER TABLE automation_rule ADD COLUMN IF NOT EXISTS key text
  CHECK (key IS NULL OR key ~ '^[a-z][a-z0-9_]{0,62}$');

-- +goose StatementBegin
-- block_key_base is the key a name suggests: folded to plain Latin letters, digits and
-- underscores, starting with a letter, at most forty characters so that a dedupe suffix fits.
CREATE OR REPLACE FUNCTION block_key_base(name text, fallback text) RETURNS text
LANGUAGE sql IMMUTABLE SET search_path = public, pg_temp AS $$
  SELECT CASE WHEN b = '' THEN fallback ELSE b END
  FROM (
    SELECT rtrim(left(regexp_replace(regexp_replace(regexp_replace(lower(imm_unaccent(name)),
      '[^a-z0-9]+', '_', 'g'), '^[^a-z]+', ''), '_+$', ''), 40), '_') AS b
  ) folded
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION saved_view_assign_key() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  base text;
  candidate text;
  n integer := 1;
BEGIN
  IF NEW.key IS NOT NULL THEN
    RETURN NEW;
  END IF;
  base := block_key_base(NEW.name, 'view');
  candidate := base;
  WHILE EXISTS (SELECT 1 FROM saved_view
                WHERE tenant_id = NEW.tenant_id AND scope_type = NEW.scope_type
                  AND scope_id IS NOT DISTINCT FROM NEW.scope_id
                  AND key = candidate AND id <> NEW.id) LOOP
    n := n + 1;
    candidate := base || '_' || n::text;
  END LOOP;
  NEW.key := candidate;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION automation_rule_assign_key() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  base text;
  candidate text;
  n integer := 1;
BEGIN
  IF NEW.key IS NOT NULL THEN
    RETURN NEW;
  END IF;
  base := block_key_base(NEW.name, 'rule');
  candidate := base;
  WHILE EXISTS (SELECT 1 FROM automation_rule
                WHERE tenant_id = NEW.tenant_id AND scope_type = NEW.scope_type
                  AND scope_id IS NOT DISTINCT FROM NEW.scope_id AND deleted_at IS NULL
                  AND key = candidate AND id <> NEW.id) LOOP
    n := n + 1;
    candidate := base || '_' || n::text;
  END LOOP;
  NEW.key := candidate;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION container_assign_handle() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.type IN ('HUB', 'COLLECTION') AND NEW.handle IS NULL THEN
    NEW.handle := container_free_handle(NEW.tenant_id, container_handle_base(NEW.name), NEW.id);
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- The existing rows, one at a time for the reason migration 0204 gives: a single UPDATE would not
-- see the keys it has just given.
-- +goose StatementBegin
DO $$
DECLARE
  row record;
BEGIN
  FOR row IN SELECT id, tenant_id, name FROM container
             WHERE type = 'HUB' AND handle IS NULL ORDER BY created_at, id LOOP
    UPDATE container SET handle = container_free_handle(row.tenant_id, container_handle_base(row.name), row.id)
    WHERE id = row.id;
  END LOOP;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS saved_view_key ON saved_view;
CREATE TRIGGER saved_view_key BEFORE INSERT OR UPDATE OF key ON saved_view
  FOR EACH ROW EXECUTE FUNCTION saved_view_assign_key();
DROP TRIGGER IF EXISTS automation_rule_key ON automation_rule;
CREATE TRIGGER automation_rule_key BEFORE INSERT OR UPDATE OF key ON automation_rule
  FOR EACH ROW EXECUTE FUNCTION automation_rule_assign_key();

-- Setting the key to NULL draws it through the trigger, oldest first.
-- +goose StatementBegin
DO $$
DECLARE
  row record;
BEGIN
  FOR row IN SELECT id FROM saved_view WHERE key IS NULL ORDER BY created_at, id LOOP
    UPDATE saved_view SET key = NULL WHERE id = row.id;
  END LOOP;
  FOR row IN SELECT id FROM automation_rule WHERE key IS NULL ORDER BY created_at, id LOOP
    UPDATE automation_rule SET key = NULL WHERE id = row.id;
  END LOOP;
END
$$;
-- +goose StatementEnd

CREATE UNIQUE INDEX IF NOT EXISTS saved_view_key_uq ON saved_view
  (tenant_id, scope_type, coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
  WHERE key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS automation_rule_key_uq ON automation_rule
  (tenant_id, scope_type, coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
  WHERE key IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (AGENTS.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
