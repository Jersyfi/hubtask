# ADR-0026 — How the query DSL turns into SQL

**Status:** accepted · **Date:** 2026-08-20

## Context

B-12 adds `QueryItems` (`POST /items:query`), the one endpoint that serves list, board and timeline
(api-guidelines.md §3). Its request carries a filter tree — a field, an operator and a value, combined with
`AND`, `OR` and `NOT` — a sort list, a grouping and a page. The set of possible requests is open, so the SQL
that answers them cannot be one statement written in advance.

That collides with the wording of rule 9 in [CLAUDE.md](../../CLAUDE.md):

> SQL only parameterised, through sqlc. Never string concatenation to build a query, not even for filters.

The rule cites ADR-0015 and threat T-06. T-06 in [security.md](../architecture/security.md) is about the same
feature and prescribes its shape:

> SQL injection through filter expressions | Exclusively parameterised queries (`sqlc`); the DSL produces an
> AST → parameters, never string concatenation; importing `fmt.Sprintf` into query building is forbidden via
> `depguard` | Lint + fuzz

So the threat model already anticipated the query DSL and answered it — an AST that yields parameters — while
the one-line summary of the rule reads as if no SQL may ever be assembled at run time. The two have to be made
to say the same thing before the first filter is compiled, because whichever is taken literally decides the
architecture of the endpoint.

The distinction that matters is not *whether* SQL text is assembled but *where the text comes from*. A query
built as `"WHERE " + field + " " + op + " '" + value + "'"` takes its text from the request. A query built by
looking a validated field up in a fixed table and emitting the constant fragment that belongs to it takes its
text from the binary. Both concatenate strings; only one of them can be made to say something the author did
not write.

## Options

**A. One sqlc query with a fixed superset of optional predicates.** Every filterable field gets an
`sqlc.narg` and a `($n IS NULL OR column = $n)` clause. Fully static SQL, generated and type-checked by sqlc.
Rejected: it expresses a conjunction of equalities and nothing else. `AND`/`OR`/`NOT` over a tree, `BETWEEN`,
`CONTAINS_ANY` over a join table and `MATCHES` cannot be written this way at all, and each new field widens
one statement that every list in the product then plans.

**B. The filter as `jsonb`, interpreted by the query itself.** One static statement takes the tree as a
parameter and walks it in SQL. The text is constant and the injection surface is nil. Rejected: no predicate
derived from a jsonb walk can use an index, so every query becomes a sequential scan of the tenant's items —
the cost B-12 is explicitly required to bound. It also moves the grammar, its validation and its error
messages into PL/pgSQL, out of reach of the domain model and of every test that is not a database test.

**C. A compiler from the validated AST to constant fragments plus parameters.** The one T-06 names. The
grammar is validated in `core/domain/model/view` against a closed catalogue of fields and the operators each
one permits; the compiler in `infrastructure/postgres` then walks that validated tree and, for each node,
emits a fragment selected by a `switch` over typed constants. Every value becomes a `$n` placeholder.

## Decision

**Option C is accepted, with the boundary written down here rather than left to each reviewer's judgement.**

Rule 9 stands unchanged in what it forbids. It is restated as: **no byte that arrived in a request may ever
become SQL text.** Concretely, for the query path:

1. **The vocabulary is closed.** Field names, operators, sort directions, null placement and the grouping
   field are parsed into Go types in the domain layer before the compiler sees them. A field is a
   `view.Field` looked up in a table declared in the source; an operator is a `view.Operator` constant.
   Anything not in the catalogue is refused with `422` and a message code — it never reaches the adapter.
2. **The compiler emits only literals from the binary.** Every fragment it appends is a constant `string` in
   the source or the result of appending one. It never interpolates a name, an operator or a value, and the
   package may not import `fmt` — enforced by a `depguard` rule, as T-06 asks.
3. **Every value is a parameter.** Values, including the identifiers inside `IN` and the search text of
   `MATCHES`, are appended to an argument slice and referenced as `$n`. The count of emitted placeholders and
   the length of that slice are asserted on every compilation, so a fragment that forgot its argument fails
   loudly rather than shifting every later parameter by one.
4. **A fuzz gate proves it.** `FuzzCompile` feeds arbitrary request documents through the parser and the
   compiler and asserts, for anything that compiles, that the SQL is drawn from the closed vocabulary, that
   placeholders and arguments agree, and that no fragment of the input appears in the SQL text. It runs in
   `make gate-fuzz` (nightly) and as a seeded unit test in `make gate-unit`.
5. **sqlc keeps everything else.** The compiler exists for the filter, the sort and the grouping of this one
   endpoint. Every other statement in the system, this endpoint's `COUNT` included, stays a sqlc query — a
   second hand-built statement anywhere else is a review finding, not a precedent.

The cost of the query is bounded by the grammar rather than by hope: a scope is mandatory, so no query is
unanchored; nesting is capped at five levels and fifty nodes (api-guidelines.md §3); the page size is capped
at 200 and the per-group limit likewise. Behind all of them sits the interactive `statement_timeout` the pool
already sets (`infrastructure/postgres/Pool.go`), which is what makes an expensive-but-legal filter slow and
finite rather than unbounded.

## Consequences

* Rule 9's row in `CLAUDE.md` gains a pointer to this ADR, so the next reader of the one-line summary finds
  the boundary rather than a contradiction.
* `core/domain/model/view` owns the grammar: what may be filtered, sorted and grouped, and with which
  operators. `/meta/capabilities` answers `query_fields` from that same catalogue, so a client configures
  itself from what the server will actually accept (api-guidelines.md §3, "only fields from
  `/meta/capabilities`").
* The adapter runs its compiled statement on the transaction the unit of work opened, exactly as the
  generated code does. Rule 3 is untouched: there is no second path to the pool, and row level security
  applies to the compiled statement like any other.
* Adding a filterable field is a change in two places — the catalogue in the domain and the column mapping in
  the compiler — and neither of them takes text from a request. Adding one is therefore a review of a table,
  which is the point.
* A future `SearchItems` (domain-model.md §5) and the saved views of `0.5.0` reuse the same AST. They must
  not grow a second compiler.

## Notes

Related: [ADR-0015](./ADR-0015-security-baseline.md) (the gates that enforce this),
[ADR-0004](./ADR-0004-api-first-openapi.md) (the contract is the source),
[ADR-0010](./ADR-0010-multi-tenancy.md) (the transaction wrapper the statement runs on),
[security.md](../architecture/security.md) T-06, [api-guidelines.md](../architecture/api-guidelines.md) §3.
Raised while implementing B-12 (issue #47).
