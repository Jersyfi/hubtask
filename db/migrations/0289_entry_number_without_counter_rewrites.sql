-- An entry that arrives with its number - a restore, a bulk load - no longer rewrites its
-- collection's counter row (migration 0204). One transaction that wrote thousands of entries into
-- one collection rewrote that row once per entry; every version stays until the transaction ends,
-- and each later write and every foreign-key check on the collection walked the whole chain - a
-- cost that grew with the square of the entries. The capacity run's seed of 10⁶ entries did not
-- finish within thirty minutes because of it (observability-reliability.md §13.2).
--
-- Instead the draw catches up: a number drawn for an entry that carries none is one past the
-- larger of the counter and the highest number the collection already holds, read from
-- work_item_number_idx. So no number is repeated, and the counter self-heals after a load that
-- never touched it.
--
-- Expand only (rule 12): the same function replaced in place, the trigger unchanged; the previous
-- binary writes the same columns and gets the same numbers.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_assign_number() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.collection_id IS NOT DISTINCT FROM OLD.collection_id THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'INSERT' AND NEW.number IS NOT NULL THEN
    RETURN NEW;
  END IF;
  UPDATE container SET item_counter = greatest(
           item_counter,
           coalesce((SELECT max(w.number) FROM work_item w WHERE w.collection_id = NEW.collection_id), 0)
         ) + 1
  WHERE id = NEW.collection_id
  RETURNING item_counter INTO NEW.number;
  RETURN NEW;
END
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
