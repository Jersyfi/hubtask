-- A short name per collection and a running number per entry (ADR-0094): the handle is how a mail
-- subject, a plus address or a spoken sentence names a collection ("#einkauf"), and the number with
-- it is an entry's ticket key ("EINKAUF-42") - the reference people quote in a reply.
--
-- Both are assigned by the database, so that every writer - the create, the copy, the import, a
-- restore - gets them without each having to remember: the handle is derived from the name when a
-- collection is inserted without one, and the number is drawn from the collection's own counter
-- when an entry is inserted or moved into another collection.
--
-- A handle is unique per workspace over every collection, trashed ones included: a handle that once
-- received mail must not quietly start delivering into a new collection that happens to share the
-- name. It is freed only when the collection is purged. A rename keeps it - an address that changes
-- with the title is an address nobody can rely on.

-- +goose Up
ALTER TABLE container ADD COLUMN IF NOT EXISTS handle text
  CHECK (handle IS NULL OR handle ~ '^[a-z][a-z0-9]{1,19}$');
ALTER TABLE container ADD COLUMN IF NOT EXISTS item_counter bigint NOT NULL DEFAULT 0;
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS number bigint;

-- +goose StatementBegin
-- container_handle_base is the handle a name suggests: folded to plain Latin letters and digits,
-- starting with a letter, at most sixteen characters so that a dedupe suffix still fits.
CREATE OR REPLACE FUNCTION container_handle_base(name text) RETURNS text
LANGUAGE sql IMMUTABLE SET search_path = public, pg_temp AS $$
  SELECT CASE
    WHEN length(b) >= 2 THEN b
    WHEN length(b) = 1 THEN b || 'x'
    ELSE 'liste'
  END
  FROM (
    SELECT left(regexp_replace(regexp_replace(lower(imm_unaccent(name)), '[^a-z0-9]', '', 'g'),
                               '^[0-9]+', ''), 16) AS b
  ) folded
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- container_free_handle is the first of base, base2, base3 ... no collection of the workspace holds.
CREATE OR REPLACE FUNCTION container_free_handle(tenant uuid, base text, self uuid) RETURNS text
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  candidate text := base;
  n integer := 1;
BEGIN
  WHILE EXISTS (SELECT 1 FROM container
                WHERE tenant_id = tenant AND handle = candidate AND id <> self) LOOP
    n := n + 1;
    candidate := base || n::text;
  END LOOP;
  RETURN candidate;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION container_assign_handle() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF NEW.type = 'COLLECTION' AND NEW.handle IS NULL THEN
    NEW.handle := container_free_handle(NEW.tenant_id, container_handle_base(NEW.name), NEW.id);
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- work_item_assign_number draws the next number of the entry's collection. An entry that arrives
-- with one - a restore of an export - keeps it, and the counter moves past it so no later entry
-- repeats it. Moving into another collection draws anew there: a key is "this collection's 42nd".
CREATE OR REPLACE FUNCTION work_item_assign_number() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.collection_id IS NOT DISTINCT FROM OLD.collection_id THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'INSERT' AND NEW.number IS NOT NULL THEN
    UPDATE container SET item_counter = greatest(item_counter, NEW.number)
    WHERE id = NEW.collection_id;
    RETURN NEW;
  END IF;
  UPDATE container SET item_counter = item_counter + 1
  WHERE id = NEW.collection_id
  RETURNING item_counter INTO NEW.number;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

-- The existing collections and entries, numbered in the order they were made.
-- One row at a time rather than one statement: a single UPDATE would not see the handles it has
-- just given, and two "Einkauf" collections would both come out as "einkauf".
-- +goose StatementBegin
DO $$
DECLARE
  row record;
BEGIN
  FOR row IN SELECT id, tenant_id, name FROM container
             WHERE type = 'COLLECTION' AND handle IS NULL ORDER BY created_at, id LOOP
    UPDATE container SET handle = container_free_handle(row.tenant_id, container_handle_base(row.name), row.id)
    WHERE id = row.id;
  END LOOP;
END
$$;
-- +goose StatementEnd

WITH numbered AS (
  SELECT id, row_number() OVER (PARTITION BY collection_id ORDER BY created_at, id) AS n
  FROM work_item WHERE number IS NULL
)
UPDATE work_item wi SET number = numbered.n FROM numbered WHERE wi.id = numbered.id;

UPDATE container c SET item_counter = coalesce(
  (SELECT max(number) FROM work_item WHERE collection_id = c.id), 0)
WHERE c.type = 'COLLECTION';

CREATE UNIQUE INDEX IF NOT EXISTS container_handle_uq ON container (tenant_id, handle)
  WHERE handle IS NOT NULL;
CREATE INDEX IF NOT EXISTS work_item_number_idx ON work_item (collection_id, number);

DROP TRIGGER IF EXISTS container_handle ON container;
CREATE TRIGGER container_handle BEFORE INSERT ON container
  FOR EACH ROW EXECUTE FUNCTION container_assign_handle();
DROP TRIGGER IF EXISTS work_item_number ON work_item;
CREATE TRIGGER work_item_number BEFORE INSERT OR UPDATE OF collection_id ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_assign_number();

-- +goose Down
DROP TRIGGER IF EXISTS work_item_number ON work_item;
DROP TRIGGER IF EXISTS container_handle ON container;
DROP FUNCTION IF EXISTS work_item_assign_number();
DROP FUNCTION IF EXISTS container_assign_handle();
DROP FUNCTION IF EXISTS container_free_handle(uuid, text, uuid);
DROP FUNCTION IF EXISTS container_handle_base(text);
DROP INDEX IF EXISTS work_item_number_idx;
DROP INDEX IF EXISTS container_handle_uq;
ALTER TABLE work_item DROP COLUMN IF EXISTS number;
ALTER TABLE container DROP COLUMN IF EXISTS item_counter;
ALTER TABLE container DROP COLUMN IF EXISTS handle;
