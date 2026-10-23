-- When an entry entered its collection's first column and its done column: what a flow's figure
-- measures its average time from first to last column by (UC-WRK-31 check 4, domain-model.md §2).
--
-- Nothing recorded these moments before. The history keeps a move's columns only for a type whose
-- history is full - a compact one keeps the verb alone - and it does not say which column an entry
-- was created into; the change log is the offline protocol's, pruned by its own retention. So the
-- figure counts only what is recorded from this migration on: an entry has a time through the flow
-- when both moments are set, and existing entries start without them.
--
-- `flow_started_at` is the first time the entry entered the first column since it came into its
-- collection, and stays when the entry is moved back there. `flow_finished_at` is the last time it
-- entered a column that completes entries. Moving to another collection clears both: that board's
-- first column is another one.
--
-- Written by a trigger rather than by each statement that places an entry, because a column is set
-- by six of them - creating, copying, editing, moving, a drag between columns, a deleted column's
-- entries taking another - reached from the use cases and from the push path alike; one that forgot
-- it would silently drop entries from the figure. The moment is the row's own `created_at` or
-- `updated_at`, which the application sets from its clock (rule 4), never the database's time. A
-- row written with the moments given - a backup being restored - keeps them; a row restored from
-- an archive older than this migration is recorded as it is inserted, like a new one. The
-- server's alone, like `completion_count`: no device writes them (offline-sync.md §4.2).
--
-- Expand only and safe for a rolling update (rule 12): two nullable columns are a catalogue change,
-- and a pod of the previous release neither reads nor writes them - its writes are recorded by the
-- trigger like any other.

-- +goose Up
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS flow_started_at timestamptz;
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS flow_finished_at timestamptz;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_flow_moments() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  moment timestamptz;
  first_column uuid;
  completes boolean;
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.flow_started_at IS DISTINCT FROM OLD.flow_started_at
       OR NEW.flow_finished_at IS DISTINCT FROM OLD.flow_finished_at THEN
      -- The statement wrote the moments itself: a restore overwriting the row.
      RETURN NEW;
    ELSIF NEW.collection_id IS DISTINCT FROM OLD.collection_id THEN
      NEW.flow_started_at := NULL;
      NEW.flow_finished_at := NULL;
    ELSIF NEW.bucket_id IS NOT DISTINCT FROM OLD.bucket_id THEN
      RETURN NEW;
    END IF;
    moment := NEW.updated_at;
  ELSE
    moment := NEW.created_at;
  END IF;
  IF NEW.bucket_id IS NULL THEN
    RETURN NEW;
  END IF;

  -- The board's first column as a board is answered: by rank, then identifier (ListBuckets).
  SELECT b.id INTO first_column
    FROM bucket b
   WHERE b.tenant_id = NEW.tenant_id AND b.collection_id = NEW.collection_id AND b.deleted_at IS NULL
   ORDER BY b.order_key COLLATE "C", b.id
   LIMIT 1;
  SELECT b.is_done_bucket INTO completes
    FROM bucket b
   WHERE b.tenant_id = NEW.tenant_id AND b.id = NEW.bucket_id;

  IF NEW.bucket_id = first_column AND NEW.flow_started_at IS NULL THEN
    NEW.flow_started_at := moment;
  END IF;
  IF completes AND (TG_OP = 'UPDATE' OR NEW.flow_finished_at IS NULL) THEN
    NEW.flow_finished_at := moment;
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS work_item_flow_moments ON work_item;
CREATE TRIGGER work_item_flow_moments
  BEFORE INSERT OR UPDATE OF bucket_id, collection_id ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_flow_moments();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
