# ADR-0064 — The one read that may ask the whole workspace a narrowed question

**Status:** proposed · **Date:** 2026-09-22

## Context

Two screens of milestone F10 asked the product a question it cannot answer, and the same wall
stopped both. The wall is deliberate, it is written down in two places, and the two places were
written to point at each other — which is why this is a decision rather than a defect.

**What the product promises.** `arc42.md` §1's functional requirements name F-06 *Views* — list,
kanban and timeline as saved, server-defined views — and F-07 *Filtering & sorting*, "a generic,
composable query DSL over every item field". Both are about a **view of a container**. No
requirement names a personal list across the workspace, and none names a search that can be
narrowed. The two reads the product has were built to those requirements and to nothing else:

* **`POST /items:query`** — the one endpoint behind list, board and timeline
  ([api-guidelines.md](../architecture/api-guidelines.md) §3). Its scope is **required**, and
  `core/domain/model/view/QuerySpec.go` says why:

  > Required, and exactly one of the two. An unanchored query is a scan of every item a tenant
  > has, and worse than that it is a question authorisation cannot answer in one step: a
  > membership is held on a hub or a collection, so "everything I may see" would be a permission
  > check per row. Naming the anchor makes the check one question with one answer, and a refusal a
  > refusal rather than a short page.

* **`POST /search`** — the full-text read (C-08). Its **words are required**, and
  `core/domain/model/view/Search.go` says why:

  > An empty search is not "everything": that is what the query endpoint answers, with a scope and
  > an order somebody chose. Ranking a whole collection by how well it matches nothing would be a
  > list in an arbitrary order.

Each closes the door on the other's side, on purpose. What falls between them is **a narrowed
question about the whole workspace**: words with a filter, or a filter with no words at all.

**What asked for it.** The owner's walk of 2026-09-22 asked for two things that are the same
thing. The search should have "direkte Filtermöglichkeiten" — a search one can narrow by where,
by label, by who, by state — and the first destination should be worth arriving at rather than a
second list of the hubs. [ADR-0063](./ADR-0063-navigation-and-the-working-surface.md) decisions 1
and 4 wrote both down; building them found that neither read answers them:

| Wanted | `items:query` | `/search` |
|---|---|---|
| words across the workspace | needs an anchor | **yes** |
| words narrowed by label, assignee, state | needs an anchor | no filter exists |
| "assigned to me, overdue", anywhere | needs an anchor | words are required |

**What the exception already costs, three times.** The per-row narrowing the scope rule avoids is
not a new idea in this system; it is what three reads already do, deliberately: `ListTrash` (C-04),
`SearchItems`, and `SuggestDuplicates`, whose own note says its rows "come from everywhere at once,
exactly as a search's do". The pattern has a name in the source — read, then narrow to what the
actor may see — and a known consequence: the page may be short rather than the refusal a scoped
read gives.

**What the schema already holds.** `db/schema.sql` carries
`wi_assignee_idx ON work_item (tenant_id, assignee_id, is_completed, due_at) WHERE deleted_at IS NULL`.
"What is assigned to me, still open, by when it is due" is an indexed read of the whole workspace
today. Nothing in the product asks it.

## Options

**A. Nothing changes.** The overview is built from what exists — the jumble's count, what this
device opened last, the saved views somebody made, the hubs for a workspace with none — and the
search keeps words alone. Cheapest, no core change, and defensible: a saved view *is* the
product's answer to "the list I look at", and one saved in a collection with `assignee_id = @me`
is "my work here". Rejected as the whole answer, because it leaves "where is this, anywhere" a
question that cannot be narrowed — and narrowing a search is the ordinary thing a person does
when the first search returns forty rows.

**B. The query gains a workspace scope** (`scope: { workspace: true }`). Rejected, and this is the
one option that must be rejected loudly: it undoes `QuerySpec.go`'s reasoning at the exact point
it was written for. Every list, board and timeline in the product would then plan an endpoint that
*can* be unanchored, and the authorisation shortcut that makes a refusal a refusal would hold for
some requests and not others. A rule that holds "usually" is not one a reviewer can apply.

**C. The search gains the filter, and its words become optional when one is present (chosen).**
One read, already the one the product means by "anywhere", already answered read-then-narrow,
already bounded and already saying that its page may be short. It gains the same validated filter
tree the query compiles, with the same closed vocabulary and the same caps, and nothing else.

**D. A use case per question** — `ListMyOverdueWork`, and the next one, and the one after.
Rejected: a catalogue that grows by one entry per screen is the shape ADR-0026 replaced when it
made one endpoint serve three views.

## Decision

**Option C.** `POST /search` becomes the one read that may ask the whole workspace a narrowed
question. Precisely:

1. **It gains `filter`**, the filter tree `Spec` already carries — the same closed field
   vocabulary from `/meta/capabilities`, the same operators, the same depth of 5, the same 50
   nodes, the same cost estimate capped at 50. It is the same parser and the same compiler; a
   second grammar is not created.
2. **`words` becomes optional when a filter is present.** A search with neither words nor a filter
   stays refused by `search.words_required` — "everything" is still not a question this API
   answers.
3. **Ordering.** With words, `ts_rank_cd` as today, because ranking is what a search is. Without
   words there is nothing to rank, so the caller may send the query's own `sort`, and the default
   is `due_at ASC NULLS LAST, id ASC` — a filtered workspace read with no words is a work list,
   and a work list is ordered by when it is due. The implicit final `id ASC` keeps the cursor
   stable, as it does everywhere.
4. **The narrowing does not change.** Read, then narrow to what the actor may see, exactly as the
   search and the trash do now; the page may be short and the contract already says so.
5. **What it does not gain**, so that this stays one read and not a second query endpoint:
   `group_by` (a board of a whole workspace is not a thing anybody drew), `expand: children` (a
   tree does not span collections), and `count: exact` (a total over a narrowed-per-row read is a
   number that costs a second pass and means less than it looks).
6. **The manifest publishes it.** `/meta/capabilities` says that the search takes a filter, so a
   client can tell rather than probe — the rule this client already lives by.

### What it makes possible, named so the decision can be judged by them

* **The search page of F10-05** narrows by where, label, who, state, when and type: each chip is a
  `FilterNode`, and the query string carries them, so a search is a link.
* **The overview of F10-04** asks one question — `assignee_id EQ @me`, `is_completed EQ false`,
  `due_at LTE @today` — with no words, and gets the answer from `wi_assignee_idx`.
* **Nothing else changes.** `items:query` keeps its anchor and ADR-0026 stands in full.

## Consequences

* A contract change, so the order is the one rule 11 gives: `api/openapi.yaml` first, then
  `make generate`, then the domain, the compiler, the use case and the tests.
* `view.Search` gains a filter and a sort; `ParseSearch` learns that empty words are allowed with
  a filter beside them. The cost estimate and the field catalogue move from `Spec`'s parser into
  something both call — one grammar, two readers.
* The `MATCHES` operator now exists in two places that mean the same thing: as a filter field on a
  scoped query, and as the words of a search. That is not a duplication to remove — one ranks and
  the other filters — but the difference belongs in `api-guidelines.md` §3 in one sentence.
* **A risk worth stating.** A filter with no words is a workspace-wide read narrowed per row, which
  is the cost `QuerySpec.go` declined for the query. It is accepted here for the reason it was
  accepted three times already, and bounded by the same cost cap; what it is *not* is a precedent
  for the query endpoint.
* Milestones: F10-04 and F10-05 wait on this ADR. F10-03's archive does not — it lists containers,
  which `GET /containers` answers with `include_archived`, and it says in a sentence that archived
  entries stay in the list they belong to.

### Backlog impact

| Work package | Target |
|---|---|
| The contract, the domain, the compiler and the use case | a core task, cut when this ADR is accepted |
| F10-04 and F10-05 | [`backlog/milestone-F10.md`](../backlog/milestone-F10.md), after it |

## Notes

Related: [ADR-0026](./ADR-0026-query-dsl-sql-construction.md) (what the filter compiles to, and
why the vocabulary is closed), [ADR-0034](./ADR-0034-language-dependent-search.md) (what the words
are read under), [ADR-0063](./ADR-0063-navigation-and-the-working-surface.md) (the two screens
that asked), `domain-model.md` §5 and §3.2, `api-guidelines.md` §3, `multi-tenancy.md` §4 (the
cost cap), `security.md` T-06.
