-- The search document keeps the word forms as well as the stems (ADR-0066, amends ADR-0034).
--
-- ADR-0034 built the document under **one** configuration: the one the entry's `content_language`
-- resolves to. The query is parsed under the *searcher's* configuration, OR-ed with `simple`. Where
-- the two differ, a word somebody typed is stemmed into something the document does not carry, and
-- the answer is "nothing matches" about an entry that is plainly there. That asymmetry is the whole
-- reason the client grew a language picker, a widening and a badge saying which language found a
-- hit - three controls working around one missing lexeme.
--
-- Measured before it was written (`concept/search/proof/language.sql`): of eight cross-language
-- searches, four find nothing today and none fails with the copy below. A German entry indexed as
-- `german` alone holds `rechnung` and not `rechnungen`, so an English reader typing what is on the
-- screen finds nothing; with both, `Rechnung` still finds it by the stem and `Rechnungen` by the
-- form.
--
-- **What it costs.** The document roughly doubles - six lexemes to fourteen on a short entry - and
-- the GIN index with it, because `simple` keeps the stop words a language's configuration drops.
-- The weights are unchanged: the added copy is `A` for the title and `B` for the notes, the same as
-- the stemmed one, so `ts_rank_cd` still ranks a title hit at 1.0000 against 0.4000 for one in a
-- note. A copy at `C`/`D` would have ranked an exact word in a title *below* a stem buried in a
-- note, which is the ordering the weights exist to prevent.
--
-- **Catalogue only, and no backfill.** Three functions are replaced and nothing is written: no
-- table is touched, no lock is held beyond a catalogue update, and the rolling update rule 12 asks
-- about is untouched. Every existing row becomes *stale* instead, which is the machinery M-09
-- already built - `search_configuration` no longer matches what builds a document today, so
-- `ReindexSearch` finds exactly those rows and rewrites them in batches, as a job, where an
-- administrator asks. A backfill here would be the rewrite that use case exists to make selective.
--
-- **During a rolling update** the previous version's pods compare `search_configuration` against
-- `hubtask_text_config(...)` rather than the recipe, so they see every newly written row as stale.
-- That is a count being pessimistic, not a row being wrong: an old pod's rebuild writes the new
-- document (the function is replaced for everybody) under the old name, and the new pods rewrite it
-- once more. It converges, and nothing is lost on the way.

-- +goose Up

-- What builds a document, named. `german+simple` rather than `german`, because the name is what
-- staleness is decided by: a row whose stored name differs from what this function answers today
-- was built by a recipe this installation has moved on from - whether the configuration changed
-- (M-09's case) or the recipe did (this one).
--
-- An entry whose language resolves to `simple` gets no second copy and keeps the plain name, so
-- those rows are not stale and are not rewritten: their document does not change.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_search_recipe(language text) RETURNS text
  LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT CASE WHEN cfg = 'simple'::regconfig THEN 'simple' ELSE cfg::text || '+simple' END
    FROM hubtask_text_config(language) AS cfg
$$;
-- +goose StatementEnd

-- The document: the entry's own configuration, and the word forms beside it.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION hubtask_search_document(language text, title text, notes text)
  RETURNS tsvector LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT setweight(to_tsvector(cfg, coalesce(title, '')), 'A')
      || setweight(to_tsvector(cfg, coalesce(notes, '')), 'B')
      || CASE WHEN cfg = 'simple'::regconfig THEN ''::tsvector
              ELSE setweight(to_tsvector('simple', coalesce(title, '')), 'A')
                || setweight(to_tsvector('simple', coalesce(notes, '')), 'B')
         END
    FROM hubtask_text_config(language) AS cfg
$$;
-- +goose StatementEnd

-- And the trigger records the recipe rather than the configuration.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION work_item_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.notes);
  -- A row whose stored recipe differs from what hubtask_search_recipe() answers today is stale,
  -- and the reindex rewrites exactly those (ADR-0034 as amended by ADR-0066).
  NEW.search_configuration := hubtask_search_recipe(NEW.content_language);
  RETURN NEW;
END $$;
-- +goose StatementEnd
