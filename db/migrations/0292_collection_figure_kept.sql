-- A collection's figure kept between reads (UC-WRK-29, S-12 of the 10⁶ capacity run): counted on
-- every read, the hub's level cost about 75 ms of database time at 56 745 entries per collection,
-- and eight concurrent readers waited 254 ms at P95 - over the 200 ms read target.
--
-- A figure is the reader's (their day, their week, what the structure in force offers), so no
-- single counter per collection can hold it. What is kept is each count with the basis it was
-- counted on, beside the collection's number of changes at that moment; a kept count answers only
-- while that number is unchanged. The number is right by construction: every write to an entry, a
-- board column or a custom field definition records that its collection changed, by trigger, in
-- the writing transaction - so every path counts (use cases, sync, restore, import, an older
-- binary in a rolling update), and the change becomes visible exactly when the write does.
--
-- Why a row per transaction rather than a counter per collection: a counter would be one row that
-- every writer of a collection updates and locks until it commits, and the seed of 0289 showed what
-- a row rewritten per entry costs. A writing transaction inserts one row per collection it touches,
-- under a token of its own, and nothing else; no two writers wait on each other. The recount job
-- folds the rows into `collection_figure_stamp` in one statement, which leaves the sum unchanged.
-- A transaction's token is drawn from a sequence, never its xid: a restored cluster starts its xids
-- again, and a token met twice would hide a change.
--
-- A write made outside its workspace's context (a cascade from another tenant's deletion, a repair
-- by a role without `app.tenant_id`) records nothing: row level security would refuse the row, and
-- the write must not fail for its bookkeeping. The recount job corrects what such a write leaves
-- behind (figure.recount). The workspace-wide definitions are recorded under the workspace's own
-- identifier, which every collection's number includes.
--
-- Derived and not archived (tenant-export.md §6): a restore writes its entries, and every write
-- records itself. Expand only (rule 12): three new tables and triggers a pod of the previous release
-- writes through like any other.
-- +goose Up
CREATE SEQUENCE IF NOT EXISTS collection_figure_touch_token;
GRANT USAGE ON SEQUENCE collection_figure_touch_token TO hubtask_app;

CREATE TABLE IF NOT EXISTS collection_figure_touch (
  tenant_id     uuid NOT NULL,
  collection_id uuid NOT NULL,   -- the collection, or the workspace's id for what all of them read
  token         bigint NOT NULL,
  PRIMARY KEY (tenant_id, collection_id, token)
);
CREATE TABLE IF NOT EXISTS collection_figure_stamp (
  tenant_id     uuid NOT NULL,
  collection_id uuid NOT NULL,
  folded        bigint NOT NULL DEFAULT 0 CHECK (folded >= 0),
  PRIMARY KEY (tenant_id, collection_id)
);
CREATE TABLE IF NOT EXISTS collection_figure (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id uuid NOT NULL,
  kind          text NOT NULL,
  basis_key     text NOT NULL,
  basis         jsonb NOT NULL,
  changes       bigint NOT NULL CHECK (changes >= 0),
  counts        jsonb NOT NULL,
  counted_at    timestamptz NOT NULL,
  verified_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, collection_id, kind, basis_key),
  CONSTRAINT collection_figure_collection_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
-- The recount walks the longest unverified first.
CREATE INDEX IF NOT EXISTS collection_figure_verified_idx ON collection_figure (tenant_id, verified_at);

ALTER TABLE collection_figure_touch ENABLE ROW LEVEL SECURITY;
ALTER TABLE collection_figure_touch FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON collection_figure_touch;
CREATE POLICY tenant_isolation ON collection_figure_touch
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
ALTER TABLE collection_figure_stamp ENABLE ROW LEVEL SECURITY;
ALTER TABLE collection_figure_stamp FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON collection_figure_stamp;
CREATE POLICY tenant_isolation ON collection_figure_stamp
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
ALTER TABLE collection_figure ENABLE ROW LEVEL SECURITY;
ALTER TABLE collection_figure FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON collection_figure;
CREATE POLICY tenant_isolation ON collection_figure
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON collection_figure_touch TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON collection_figure_stamp, collection_figure TO hubtask_app;

-- A collection's number of changes: the folded ones and the rows not folded yet, its own and its
-- workspace's. Monotonic, and unchanged by a fold, which moves rows into the stamp in one commit.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION collection_figure_changes(collection uuid) RETURNS bigint
LANGUAGE sql STABLE SET search_path = public, pg_temp AS $$
  SELECT coalesce((SELECT sum(s.folded) FROM collection_figure_stamp s
                    WHERE s.tenant_id = current_tenant_id()
                      AND s.collection_id IN (collection, current_tenant_id())), 0)::bigint
       + (SELECT count(*) FROM collection_figure_touch t
           WHERE t.tenant_id = current_tenant_id()
             AND t.collection_id IN (collection, current_tenant_id()))
$$;
-- +goose StatementEnd
GRANT EXECUTE ON FUNCTION collection_figure_changes(uuid) TO hubtask_app;

-- Records that a collection changed, once per transaction and collection. The last collection is
-- remembered so that a statement writing thousands of entries into one collection probes nothing;
-- both settings are transaction-local and roll back with a savepoint, as the row does.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION collection_figure_touch(tenant uuid, collection uuid) RETURNS void
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  token bigint;
BEGIN
  IF collection IS NULL OR tenant IS DISTINCT FROM current_tenant_id() THEN
    RETURN;
  END IF;
  IF current_setting('hubtask.figure_last', true) = collection::text THEN
    RETURN;
  END IF;
  -- A workspace's own deletion cascades to its entries; nothing is recorded for a workspace gone.
  IF NOT EXISTS (SELECT 1 FROM tenant t WHERE t.id = tenant) THEN
    RETURN;
  END IF;
  token := nullif(current_setting('hubtask.figure_token', true), '')::bigint;
  IF token IS NULL THEN
    token := nextval('collection_figure_touch_token');
    PERFORM set_config('hubtask.figure_token', token::text, true);
  END IF;
  INSERT INTO collection_figure_touch (tenant_id, collection_id, token)
  VALUES (tenant, collection, token)
  ON CONFLICT DO NOTHING;
  PERFORM set_config('hubtask.figure_last', collection::text, true);
END
$$;
-- +goose StatementEnd

-- BEFORE rather than AFTER: an after-trigger queues an event per row until the statement ends, and
-- a load of 500 000 entries would queue as many (migration 0289).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_figure_touch() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM collection_figure_touch(OLD.tenant_id, OLD.collection_id);
    RETURN OLD;
  END IF;
  PERFORM collection_figure_touch(NEW.tenant_id, NEW.collection_id);
  IF TG_OP = 'UPDATE' AND OLD.collection_id IS DISTINCT FROM NEW.collection_id THEN
    PERFORM collection_figure_touch(OLD.tenant_id, OLD.collection_id);
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS work_item_figure_touch ON work_item;
CREATE TRIGGER work_item_figure_touch
  BEFORE INSERT OR UPDATE OR DELETE ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_figure_touch();

-- A column's limit decides FLOW's columns over their limit.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bucket_figure_touch() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM collection_figure_touch(OLD.tenant_id, OLD.collection_id);
    RETURN OLD;
  END IF;
  PERFORM collection_figure_touch(NEW.tenant_id, NEW.collection_id);
  RETURN NEW;
END
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS bucket_figure_touch ON bucket;
CREATE TRIGGER bucket_figure_touch
  BEFORE INSERT OR UPDATE OR DELETE ON bucket
  FOR EACH ROW EXECUTE FUNCTION bucket_figure_touch();

-- A definition decides which values FIELD_SUM sums; a workspace-wide one is every collection's.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION custom_field_figure_touch() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    PERFORM collection_figure_touch(OLD.tenant_id, coalesce(OLD.collection_id, OLD.tenant_id));
    RETURN OLD;
  END IF;
  PERFORM collection_figure_touch(NEW.tenant_id, coalesce(NEW.collection_id, NEW.tenant_id));
  IF TG_OP = 'UPDATE' AND OLD.collection_id IS DISTINCT FROM NEW.collection_id THEN
    PERFORM collection_figure_touch(OLD.tenant_id, coalesce(OLD.collection_id, OLD.tenant_id));
  END IF;
  RETURN NEW;
END
$$;
-- +goose StatementEnd
DROP TRIGGER IF EXISTS custom_field_figure_touch ON custom_field_definition;
CREATE TRIGGER custom_field_figure_touch
  BEFORE INSERT OR UPDATE OR DELETE ON custom_field_definition
  FOR EACH ROW EXECUTE FUNCTION custom_field_figure_touch();

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
