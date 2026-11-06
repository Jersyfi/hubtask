-- A collection's key prefix (user-experience.md §5.6): the short word its entries' keys start with,
-- "INC" in "INC-42", so that one key format reads alike in a list, a panel, a board card and a
-- mail. The handle stays the collection's plus address and spoken name (ADR-0094); the key is the
-- prefix and the number where a prefix is set, the handle in capitals and the number otherwise,
-- and a key in either form names the entry (MailIntake.sql ItemByKey).
--
-- Given by the database when a collection is inserted without one, as the handle is (0204), so
-- every writer - the create, a template, a copy, a restore of an older archive - gets one: the
-- first three letters of a one-word name, the initials of several (at most four), folded to plain
-- Latin letters. Unique in the workspace over every collection, trashed ones included, and never
-- another collection's handle in capitals - "INC-42" must name one entry, whichever of the two
-- words the collection that holds it answers to. The trigger below keeps both sides of that, and
-- the handle's own derivation skips the prefixes.
--
-- Expand only (rule 12): a nullable column the previous release neither reads nor writes, and the
-- collections made before keep none - their keys stay as people already quote them. The CHECK is
-- added NOT VALID and validated, with the unique index built CONCURRENTLY, in 0364.

-- +goose Up
ALTER TABLE container ADD COLUMN IF NOT EXISTS key_prefix text;
ALTER TABLE container DROP CONSTRAINT IF EXISTS container_key_prefix_check,
  ADD CONSTRAINT container_key_prefix_check
  CHECK (key_prefix IS NULL OR (type = 'COLLECTION' AND key_prefix ~ '^[A-Z][A-Z0-9]{1,4}$')) NOT VALID;

-- +goose StatementBegin
-- container_key_prefix_base is the prefix a name suggests.
CREATE OR REPLACE FUNCTION container_key_prefix_base(name text) RETURNS text
LANGUAGE plpgsql IMMUTABLE SET search_path = public, pg_temp AS $$
DECLARE
  words text[];
  word text;
  base text := '';
BEGIN
  words := array_remove(regexp_split_to_array(upper(imm_unaccent(coalesce(name, ''))), '[^A-Z0-9]+'), '');
  IF cardinality(words) >= 2 THEN
    FOREACH word IN ARRAY words LOOP
      word := regexp_replace(word, '^[0-9]+', '');
      IF word <> '' AND length(base) < 4 THEN
        base := base || left(word, 1);
      END IF;
    END LOOP;
  END IF;
  IF length(base) < 2 THEN
    base := left(regexp_replace(array_to_string(words, ''), '^[0-9]+', ''), 3);
  END IF;
  RETURN CASE length(base) WHEN 0 THEN 'KEY' WHEN 1 THEN base || 'X' ELSE base END;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- container_key_prefix_taken says whether another collection of the workspace holds the word as
-- its prefix or as its handle. Volatile, as the two below: a row an earlier row of the same
-- statement inserted has to be seen.
CREATE OR REPLACE FUNCTION container_key_prefix_taken(tenant uuid, candidate text, self uuid) RETURNS boolean
LANGUAGE sql SET search_path = public, pg_temp AS $$
  SELECT EXISTS (SELECT 1 FROM container
                 WHERE tenant_id = tenant AND id <> self AND type = 'COLLECTION'
                   AND (key_prefix = candidate OR handle = lower(candidate)))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- container_free_key_prefix is the first of base, base2, base3 ... that is free, shortened so the
-- number still fits in five; none past 9999, and the collection's keys are then read with its handle.
CREATE OR REPLACE FUNCTION container_free_key_prefix(tenant uuid, base text, self uuid) RETURNS text
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  candidate text := base;
  n integer := 1;
BEGIN
  WHILE container_key_prefix_taken(tenant, candidate, self) LOOP
    n := n + 1;
    IF n > 9999 THEN
      RETURN NULL;
    END IF;
    candidate := left(base, 5 - length(n::text)) || n::text;
  END LOOP;
  RETURN candidate;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- A handle is drawn past the other collections' prefixes too, so a derived handle never collides
-- with one (the rest as 0204).
CREATE OR REPLACE FUNCTION container_free_handle(tenant uuid, base text, self uuid) RETURNS text
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  candidate text := base;
  n integer := 1;
BEGIN
  WHILE EXISTS (SELECT 1 FROM container
                WHERE tenant_id = tenant AND id <> self
                  AND (handle = candidate OR key_prefix = upper(candidate))) LOOP
    n := n + 1;
    candidate := base || n::text;
  END LOOP;
  RETURN candidate;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- container_assign_key_prefix gives a new collection its prefix and refuses a prefix or a handle
-- that would make a key name two collections. The refusal is a unique violation on the index it
-- stands in for, so the adapters read it as they read a taken handle. It runs after
-- container_handle (triggers fire by name), so a new collection's own handle is already there.
CREATE OR REPLACE FUNCTION container_assign_key_prefix() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.type <> 'COLLECTION' THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'INSERT' AND NEW.key_prefix IS NULL THEN
    NEW.key_prefix := container_free_key_prefix(NEW.tenant_id, container_key_prefix_base(NEW.name), NEW.id);
    RETURN NEW;
  END IF;
  IF NEW.key_prefix IS NOT NULL
     AND (TG_OP = 'INSERT' OR NEW.key_prefix IS DISTINCT FROM OLD.key_prefix)
     AND container_key_prefix_taken(NEW.tenant_id, NEW.key_prefix, NEW.id) THEN
    RAISE EXCEPTION 'key prefix taken' USING ERRCODE = 'unique_violation', CONSTRAINT = 'container_key_prefix_uq';
  END IF;
  IF NEW.handle IS NOT NULL AND TG_OP = 'UPDATE' AND NEW.handle IS DISTINCT FROM OLD.handle
     AND EXISTS (SELECT 1 FROM container
                 WHERE tenant_id = NEW.tenant_id AND id <> NEW.id AND key_prefix = upper(NEW.handle)) THEN
    RAISE EXCEPTION 'handle taken' USING ERRCODE = 'unique_violation', CONSTRAINT = 'container_handle_uq';
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS container_key_prefix ON container;
CREATE TRIGGER container_key_prefix BEFORE INSERT OR UPDATE OF key_prefix, handle ON container
  FOR EACH ROW EXECUTE FUNCTION container_assign_key_prefix();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
