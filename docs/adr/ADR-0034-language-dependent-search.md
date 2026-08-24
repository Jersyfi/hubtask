# ADR-0034 — The language-dependent search document

**Status:** accepted · **Date:** 2026-08-24

## Context

C-08 gives the product `SearchItems`. Half of it is already in the schema: `work_item.search_vector` is a
generated column and the query language's virtual `text` field runs `MATCHES` against it
([ADR-0026](./ADR-0026-query-dsl-sql-construction.md)). What it is not is language-dependent. The column is
hard-wired to `to_tsvector('simple', …)`, which lower-cases and splits on word boundaries and does nothing
else — no German inflection, no English stemming — while [i18n-l10n.md](../architecture/i18n-l10n.md) §5
promises the opposite:

> `tsvector` with a language-dependent configuration; an item's language is detected or set
> (`item.content_language`), defaulting to the creator's locale; fallback `simple` for unsupported languages
> (CJK)

`content_language` is a column nothing writes, so the promise has nothing to stand on either.

The obstacle is exact and it is PostgreSQL's. A generated column's expression must be `IMMUTABLE`, and

```sql
to_tsvector(content_language::regconfig, title)
```

is not: the cast from `text` to `regconfig` is `STABLE`, because what a configuration name resolves to depends
on `search_path` and on the catalogue. PostgreSQL refuses the definition outright. The one-argument
`to_tsvector(text)` has the same problem for a different reason — it reads `default_text_search_config`, which
is a session setting.

So the language cannot simply be substituted into the column that exists. Something has to change about *how*
the document is maintained, and that decision has a second half: whatever is chosen has to reach a populated
`work_item` without an `ACCESS EXCLUSIVE` lock long enough to fail a rolling update
([ADR-0003](./ADR-0003-postgresql-as-single-datastore.md), rule 12).

## Options

**A. An immutable wrapper over a bounded set of configurations, still as a generated column.** A
`CASE` over the language tag, each branch calling `to_tsvector` with a *literal* configuration name, wrapped in
a function marked `IMMUTABLE`. The wrapper is then honestly immutable — a fixed input maps to a fixed
expression — and the generated column is legal.

Rejected, for two reasons that are both about operations rather than about correctness.

The first is decisive: adding a `STORED` generated column rewrites the table. `ALTER TABLE … ADD COLUMN …
GENERATED ALWAYS AS (…) STORED` takes `ACCESS EXCLUSIVE` and holds it for the whole rewrite, which on a
populated `work_item` is exactly the migration the acceptance criterion forbids. The rewrite cannot be
batched, because it is one statement; it cannot be deferred, because a generated column has no other way of
being filled. Expand/contract has no answer to it.

The second is that the bounded set has to be bounded *in the migration*. An `IMMUTABLE` function may not query
`pg_ts_config`, so the branches are literals — and a literal naming a configuration this installation does not
have (`'german'` on a PostgreSQL built without it) fails at write time, on an ordinary `INSERT`, for a row
whose only sin is a language tag. The set of stock configurations has grown between major versions and will
grow again; the support matrix is PostgreSQL 16 and 17 today.

**B. A trigger-maintained column.** A plain nullable `tsvector` column, a `BEFORE INSERT OR UPDATE` trigger
that fills it, and a resolver that maps a BCP-47 tag to a configuration this installation actually has.

**C. Compute the vector in the application and write it as a parameter.** No trigger, no generated column: the
Go code calls something that produces a `tsvector` and the `INSERT` binds it.

Rejected. It would put PostgreSQL's text search configuration into the application layer, where nothing else
about lexemes lives, and it would be wrong for every write that does not go through that code path — a
restore, a migration, an import, `RunRetention` clearing a title. A document maintained beside the row rather
than by the row is one that silently stops being maintained.

## Decision

**B.** `work_item.search_document` is a plain `tsvector` column, maintained by a trigger, indexed with GIN, and
built through a `STABLE` resolver that asks the catalogue which configurations exist.

Three pieces, and each one is a consequence of the rejection above:

* `hubtask_text_config(text) RETURNS regconfig` maps the language tag to a configuration. It is `STABLE`
  rather than `IMMUTABLE` — which a trigger permits and a generated column does not — and that is precisely
  what lets it read `pg_ts_config` and fall back to `simple` for a configuration this installation has not
  got. An installation without `german` indexes German entries as `simple` and finds them by exact word,
  rather than refusing the write.
* `hubtask_search_document(language, title, notes)` builds the vector: the title weighted `A`, the notes `B`,
  so that `ts_rank_cd` ranks a hit in a title above one buried in a note without the ranking having to know
  which column it came from.
* The trigger runs `BEFORE INSERT OR UPDATE`, so every path that writes a row maintains the document —
  including the ones that are not use cases.

The column reaches a populated table in the expand/contract shape rule 12 demands, and the shape is what the
choice bought: `ADD COLUMN` with no default is a catalogue change, the trigger makes every *new* write
correct from that moment, the backfill runs in batches in its own transactions, and the index is built
`CONCURRENTLY` afterwards. No step holds `ACCESS EXCLUSIVE` for longer than a catalogue update.

`search_vector`, the generated column that exists, is left in place by this migration and dropped by a later
one. That is the contract half, and it is not optional politeness: during a rolling update the old pods are
still selecting it.

The trigram index the same task adds is the supplement i18n-l10n.md §5 names for CJK and Thai, and it is an
expression index over title and notes rather than a second stored copy of them. A tsquery cannot find a
substring of a token, and a language whose script has no word boundaries produces one token per run of
characters — so for those scripts the trigram index is not an optimisation of the search, it *is* the search.

## Consequences

* Writing a work item costs one trigger invocation. It is one function call over two short text values, on a
  table whose writes are already a network round trip; the alternative was a table rewrite.
* A tenant's language tag never reaches SQL text. The resolver takes it as a parameter, and rule 9 is
  therefore untouched — including in the query compiler, which binds the configuration the same way
  ([ADR-0026](./ADR-0026-query-dsl-sql-construction.md)).
* `MATCHES` in the query language now reads `search_document`, whose lexemes are a language's rather than
  `simple`'s. A client that searched for an inflected form and found nothing now finds it; a client that
  relied on `simple`'s literal matching sees the stem match instead. The virtual field is unchanged, which is
  the point of it having been virtual.
* A query is parsed under the searcher's configuration and under `simple`, and the two are `OR`-ed. Parsing
  per row — the configuration of the row being searched — would be exact, and it would also make the GIN
  index unusable, because an index scan needs the query to be constant for the scan. Two constant queries are
  two index scans and a bitmap OR.
* Changing an item's language rewrites its document on the next write, not retroactively. `UPDATE … SET
  content_language` is a write, so the trigger fires; nothing else has to be remembered.
* Adding a language is a row in the resolver's mapping and a migration to rebuild the documents of the items
  that carry the tag. Neither is a code path, and both are reviewable as a table.
* `/meta/capabilities` answers which languages this installation can index, read from `pg_ts_config` through
  a port rather than from a constant. A client's language picker is then data — and an installation that lost
  a configuration says so instead of silently indexing everything as `simple`.

## Notes

Related: [ADR-0003](./ADR-0003-postgresql-as-single-datastore.md) (forward-only, rolling-update-safe migrations),
[ADR-0026](./ADR-0026-query-dsl-sql-construction.md) (the compiler that binds the configuration),
[ADR-0010](./ADR-0010-multi-tenancy.md) (the transaction the search runs on),
[i18n-l10n.md](../architecture/i18n-l10n.md) §5, [security.md](../architecture/security.md) T-04.
Raised while implementing C-08 (issue #88).
