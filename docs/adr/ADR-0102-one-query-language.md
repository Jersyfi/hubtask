# ADR-0102 — One query language: the Hubtask query, two spellings onto the filter tree

**Status:** accepted · **Date:** 2026-11-13 · **Decided:** 2026-11-12, by the owner (the concept
[work-surface](../backlog/work-surface/README.md) §6.11, E-10, approved as proposed)

**Rule lives in:** [api-guidelines.md](../architecture/api-guidelines.md) §3.1 (how a query becomes
SQL, the Hubtask query included). Amends [ADR-0026](./ADR-0026-query-dsl-sql-construction.md) and
[ADR-0066](./ADR-0066-search-is-one-question.md).

## Context

One query had four spellings: the filter tree (ADR-0026), the `=` line of the API, MCP and
`hubctl`, the `key:value` line of the search page (ADR-0066, parsed in the browser only), and CEL.
A question written on the search page could not be pasted into a view, an API call or an agent.

## Decision

* **One parser in the domain** reads the `field: value` spelling (comma or, space and, leading `-`
  not, comparisons, ranges, groups, free words, time and people words, `type:`, `sort:`) and the
  `=` line, onto the filter document and the sort; a formatter writes the `field: value` spelling
  back without loss. Keywords are English; errors are codes with the character and a suggestion.
* **The filter tree stays the only thing compiled to SQL.** The text adds no path to SQL; ADR-0026's
  boundary holds unchanged.
* **The browser has a twin**, `packages/query`, held to the parser by shared test lines; the server
  reads every text again. ADR-0066's browser-only line is superseded by it.
* **Keys are served** under `query_keys`, only those the parser reads; names are the client's.
* **Counts and value suggestions are summaries** of the line (`POST /items:aggregate` with `q`),
  computed for the reader alone.

## Alternatives

* A grammar per product: rejected by Q-1 - products bring fields, not dialects.
* Parsing only in the browser and sending the tree: a door without a browser (MCP, `hubctl`) could
  not take the text.

## Consequences

A saved view keeps its tree; a text is a spelling of it. Keys a product prefix names but the server
cannot yet answer (`plan.`, `asset.`, `page.`, `post.`, `goal.`, `time.`) are not served until they
are. Two parsers must stay alike: the shared lines are the price.
