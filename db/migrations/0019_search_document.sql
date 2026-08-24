-- The language-dependent search document (C-08, ADR-0034).
--
-- `search_vector` is a generated column over `to_tsvector('simple', …)`. It lower-cases and splits
-- on word boundaries and does nothing else, while i18n-l10n.md §5 promises a configuration chosen
-- per item from `content_language`. The substitution cannot be made in place: a generated column's
-- expression must be IMMUTABLE and the cast `text::regconfig` is STABLE, so PostgreSQL refuses the
-- definition - and the immutable-wrapper way round that would have to be *added*, which for a
-- STORED generated column means rewriting a populated table under ACCESS EXCLUSIVE.
--
-- So the document is maintained by a trigger, which is what makes the four steps below possible at
-- all: a catalogue-only ADD COLUMN, a trigger that makes every new write correct from that moment,
-- a backfill in batches with a commit each, and - in the next migration - an index built
-- CONCURRENTLY. Nothing here holds ACCESS EXCLUSIVE for longer than a catalogue update, which is
-- what a rolling update needs (CLAUDE.md rule 12, ADR-0003).
--
-- `search_vector` stays. Dropping it belongs to a later migration, because during a rolling update
-- the pods of the previous version are still selecting it - that is the contract half of
-- expand/contract, and skipping it is how a deploy takes the old replicas down with it.

-- +goose NO TRANSACTION

-- +goose Up

-- The resolver: a BCP-47 tag to one of this installation's text search configurations.
--
-- STABLE rather than IMMUTABLE, which is exactly what the trigger buys. An IMMUTABLE function may
-- not read `pg_ts_config`, so it would have to name its configurations as literals - and a literal
-- naming one this PostgreSQL was built without turns an ordinary INSERT into an error, for a row
-- whose only sin is a language tag. Here the catalogue is asked, and what it does not have falls
-- back to `simple`: an installation without `german` indexes German entries by exact word instead
-- of refusing to store them.
--
-- The mapping is the primary subtag only. `de-AT` and `de` are one configuration - a stemmer is a
-- language's, not a region's - and everything unmapped, CJK and Thai included, is `simple`.
-- The mapping, and the one place it is written down. `/meta/capabilities` answers the same
-- function, so a client's language picker is this list rather than a constant compiled into it -
-- and an installation whose PostgreSQL was built without a configuration says so instead of
-- silently indexing those entries word by word (C-08, ADR-0034).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_text_languages()
  RETURNS TABLE (tag text, configuration text) LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT m.tag, c.cfgname::text
    FROM (VALUES
      ('ar','arabic'),    ('hy','armenian'),   ('eu','basque'),      ('ca','catalan'),
      ('da','danish'),    ('nl','dutch'),      ('en','english'),     ('fi','finnish'),
      ('fr','french'),    ('de','german'),     ('el','greek'),       ('hi','hindi'),
      ('hu','hungarian'), ('id','indonesian'), ('ga','irish'),       ('it','italian'),
      ('lt','lithuanian'),('ne','nepali'),     ('nb','norwegian'),   ('nn','norwegian'),
      ('no','norwegian'), ('pt','portuguese'), ('ro','romanian'),    ('ru','russian'),
      ('sr','serbian'),   ('es','spanish'),    ('sv','swedish'),     ('ta','tamil'),
      ('tr','turkish'),   ('yi','yiddish')
    ) AS m(tag, cfgname)
    JOIN pg_ts_config c ON c.cfgname = m.cfgname
   ORDER BY m.tag
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_text_config(language text) RETURNS regconfig
  LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT coalesce(
    (SELECT l.configuration::regconfig
       FROM hubtask_text_languages() l
      WHERE l.tag = lower(split_part(btrim(coalesce(language, '')), '-', 1))
      LIMIT 1),
    'simple'::regconfig)
$$;
-- +goose StatementEnd

-- The document: the title weighted A and the notes B, so that ts_rank_cd ranks a hit in a title
-- above one buried in a note without the ranking having to know which column it came from.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_search_document(language text, title text, notes text)
  RETURNS tsvector LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT setweight(to_tsvector(hubtask_text_config(language), coalesce(title, '')), 'A')
      || setweight(to_tsvector(hubtask_text_config(language), coalesce(notes, '')), 'B')
$$;
-- +goose StatementEnd

-- A plain nullable column: no default, no generated expression, therefore a catalogue change and
-- not a rewrite.
ALTER TABLE work_item ADD COLUMN IF NOT EXISTS search_document tsvector;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.notes);
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- Every path that writes a row maintains the document, including the ones that are not use cases:
-- a restore, an import, a repair by hand. UPDATE is narrowed to the three columns the document is
-- built from, so completing a task or dragging a card recomputes nothing.
DROP TRIGGER IF EXISTS work_item_search_document ON work_item;
CREATE TRIGGER work_item_search_document
  BEFORE INSERT OR UPDATE OF title, notes, content_language ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_search_document();

-- The backfill: a keyset walk over the primary key, one transaction per batch.
--
-- Batched because the alternative is one transaction over the whole table - minutes of row locks
-- and a WAL segment per thousand rows, during which a rolling update's new pods are waiting for
-- this migration to finish. Keyset rather than `WHERE search_document IS NULL LIMIT n`, which
-- would rescan the table from the beginning for every batch.
--
-- Row level security is the reason for the branch. It is FORCE-d on `work_item`, so it applies to
-- the table's owner as well, and a migrator that is neither a superuser nor BYPASSRLS would
-- silently update nothing at all - the worst possible outcome, because a backfill that reports
-- success and wrote nothing leaves a search that answers a shorter list than the truth. Where that
-- is the case the migration gives *itself* a policy for the length of the walk and takes it away
-- again; `hubtask_app` is not the migrator and is untouched by it (multi-tenancy.md §2.1).
-- +goose StatementBegin
DO $backfill$
DECLARE
  unrestricted boolean;
  batch        uuid[];
  cursor_id    uuid := '00000000-0000-0000-0000-000000000000';
BEGIN
  SELECT rolsuper OR rolbypassrls INTO unrestricted FROM pg_roles WHERE rolname = current_user;
  IF NOT unrestricted THEN
    EXECUTE 'CREATE POLICY search_backfill ON work_item TO CURRENT_USER USING (true) WITH CHECK (true)';
  END IF;

  LOOP
    SELECT array_agg(id ORDER BY id) INTO batch
      FROM (SELECT id FROM work_item WHERE id > cursor_id ORDER BY id LIMIT 2000) AS page;
    EXIT WHEN batch IS NULL;

    UPDATE work_item
       SET search_document = hubtask_search_document(content_language, title, notes)
     WHERE id = ANY(batch);

    cursor_id := batch[array_upper(batch, 1)];
    COMMIT;
  END LOOP;

  IF NOT unrestricted THEN
    EXECUTE 'DROP POLICY search_backfill ON work_item';
  END IF;
END $backfill$;
-- +goose StatementEnd
