# ADR-0066 — Search is one question, and the document holds every word form

**Status:** accepted · **Date:** 2026-09-24 · **Accepted:** 2026-09-24

## Context

The owner asked for the search to be looked at whole: what the bar does, what the search screen can
narrow by, what Jira, Linear, GitHub and Todoist do with the same problem, and what of it fits here.
Two sentences in the brief were decisions rather than questions, and both are answered below: *"Das
muss eine Volltextsuche sein, ohne dass ich eine Sprache auswählen muss"*, and — of Jira — *"wie die
Suche in der Navigation damit verbunden ist, dass man von dort aus in den Filter springen kann"*.

Four surfaces speak about filtering in this client and they speak four languages:

| Where | What it can say |
|---|---|
| `frame/BarSearch.svelte` | words — and it **searches nothing**: it hands them to a store and navigates |
| `views/SearchView.svelte` | a field, a language picker, a meaning switch |
| `search/FilterChips.svelte` + `data/searchfilters.ts` | six chips: kind, state, when, label, whose, where |
| `entries/QueryPanel.svelte` + `QueryBuilder` + `data/query.ts` | the whole grammar — but only on a **container** |

The fourth is good and is not reachable from search. So the screen whose purpose is finding things
has the weakest filter in the product, while `POST /search` has accepted the entire `FilterNode`
grammar, the server-resolved placeholders, a `sort` and the two lifecycle flags since
[ADR-0064](./ADR-0064-the-workspace-wide-read.md). Six chips reach almost none of it.

**And the language picker is a symptom, not a control.** [ADR-0034](./ADR-0034-language-dependent-search.md)
builds an entry's document under **one** configuration — the one its `content_language` resolves to
— while the query is parsed under the *searcher's*, OR-ed with `simple`. Where the two differ, a word
somebody typed is stemmed into something the document does not carry. `data/searchlanguages.ts` (127
lines) says so itself: R-08 step 8 found the failure, and that file is the compromise reached instead
of a fix. Measured against PostgreSQL 16 before anything was written: of eight cross-language searches,
**four find nothing**. The measurement is now a test —
`TestAnEntryIsFoundByItsOwnWordsWhateverTheSearcherReads`, whose three words are the ones German and
English disagree about, and which goes red the moment the second copy is taken out of the
document.

## Decision

### 1. The document carries the entry's own configuration **and** the word forms

`hubtask_search_document` appends a `simple` copy of the title and the notes at the same `A`/`B`
weights. Nothing else changes: the query side already asks *searcher's configuration OR `simple`*.

**This amends ADR-0034**, whose rejected option C ("compute the vector in the application") and
chosen option B are untouched — what changes is what B *builds*, one line of one function.
`i18n-l10n.md` §5 gains the same sentence.

The evidence, not the argument:

* Of the eight cross-language searches measured, four fail today and none fails with the copy.
* **Stemming is kept.** A German document reads `'pruf':2A 'prüfen':4A 'rechnung':1A
  'rechnungen':3A`, so a German reader still finds *Rechnungen* by typing *Rechnung* and an English
  reader now finds it by typing what is on the screen.
* **The ranking is untouched**: a title hit scores 1.0000 against 0.4000 for one in a note, before
  and after. That is why the copy carries `A`/`B` rather than `C`/`D` — a lower copy would rank an
  exact word in a *title* below a stem buried in a *note*, the ordering the weights exist to prevent.
* An entry whose language resolves to `simple` gets no copy and is not touched: its document is
  already what this produces, including every CJK and Thai entry.

**What it costs.** The document roughly doubles — six lexemes to fourteen on a short entry — and the
GIN index with it, because `simple` keeps the stop words a language's configuration drops. That is
the whole bill.

**How it reaches a populated table.** Migration 0097 is catalogue-only: three functions replaced, no
row written, no lock beyond a catalogue update, so rule 12 is untouched. A new
`hubtask_search_recipe(language)` names what built a document — `german+simple` rather than `german`
— and the trigger stores that name. Every affected row therefore becomes **stale**, which is
precisely the machinery M-09 built: `ReindexSearch` finds exactly those and rewrites them in batches,
as a job, where an administrator asks. A backfill in the migration would be the rewrite that use case
exists to make selective.

During a rolling update the previous version's pods compare against `hubtask_text_config(…)` rather
than the recipe, so they count every newly written row as stale. That is a count being pessimistic,
not a row being wrong — an old pod's rebuild writes the new document under the old name and the new
pods rewrite it once more. It converges.

**And so the picker goes**, with the widening, the "found under" badge and `searchlanguages.ts`
entire. ADR-0063 decision 4 asked for the language's default to be "any language this workspace
holds"; this is that, reached by making the sentence true rather than by adding a control that
approximates it.

### 2. One filter language, three surfaces, one state

A search is a single string holding the words and the narrowing together — `data/searchquery.ts`:

```
Rechnungen who:me is:open in:Büro due:week
```

The chips read it and write it; the field for the words reads it and writes it; the text editor
**is** it. Nothing holds a second copy.

That is the one thing Jira and Linear both get wrong in the same way, and it is not a matter of
care: they hold the filter twice, once per surface, so Jira's basic mode cannot represent some JQL
and says so with a warning. With one representation there is nothing to fall out of step. Where the
chips have no control for something the line says — `title:`, `note:`, `sort:` — the screen **names
it and offers the way through** rather than refusing to open:

> The controls have nothing for `title:Rech` — it is still narrowing this search. Edit it as text
> to change it.

**This amends ADR-0063 decision 4** in one respect: "every filter is in the query string" becomes
**one** parameter, `?f=`, written in that language. One parameter per chip could only ever say the
six things the chips had controls for; the grammar behind it says seventeen operators. The rule the
sentence was protecting is unchanged and is what makes the parameter safe: the narrowing is
structural and travels, the words are content and never do.

### 3. A chip names what is chosen, not how many

`Status: Open` · `Kind: Task +1` · `Collection: Büro`.

**This amends ADR-0063 decision 4's** "the chip says how many are chosen". A count was a pill inside
a pill — the number carried the badge's own inset *and* the chip's, nine pixels more room after it
than before the word — and tuning the spacing would have left the other half standing: a reader who
wants to know what `Kind 2` means has to open it. Naming the first answer and counting the rest is
one text run, one type treatment, nothing nested, and a bar that can be read without being opened.
Which is what a filter bar is for.

### 4. The bar answers, and Search is a place as well as a field

The bar's field searches now. Before typing it offers the narrowings that need no words; while
typing it shows the first five hits (a `peek`: one page, and the search screen's own state is never
touched) with those narrowings offered as ways to narrow *these words*, and two ways on — **All
results**, and **All results, with the filter open**. Enter presses what is highlighted, and with
nothing highlighted it does what the menu draws the key on.

The reason the bar was inert was written in its own comment: "the debounce, the language and the
widening stay in one place". Two thirds of that is the language, and the language is gone.

> **Amended 2026-09-25.** A bar that answers while somebody types has to *match* while somebody
> types, and it did not: a tsquery compares whole lexemes, so `Ann` was not a worse match for
> *Anna's moments* — it was no match at all — and `A` and `An` were worse still, because an English
> configuration drops them as stop words and the query became empty. The owner found it on the
> first name he tried. The menu showed nothing for four of five keystrokes, which reads as a menu
> that does not work.
>
> **So the last word is matched as a beginning as well.** Only the last, and only where it is one
> plain word: a trailing space, a quote, a leading minus and `or` all finish a word, and so does
> any character a text search parser would not keep inside a token. `view.Search.PrefixTerm` is
> that rule and it is in the domain, beside `WithoutWordBoundaries`, because it is a statement
> about what somebody typed rather than about SQL.
>
> **Under `simple`, always** — which is where decision 1 pays for itself a second time. A
> configuration would stem the beginning into something that is not one and would drop a
> one-letter word entirely; `simple` does neither, and since decision 1 every document carries a
> `simple` copy of itself, so a beginning reaches an entry whatever language it was written in.
> The word is bound and the `:*` is written, so `to_tsquery` — which, unlike
> `websearch_to_tsquery`, has operators of its own — can never be handed one (rule 9, T-06).
>
> **A beginning does not rank.** It is a fourth branch of the match and no part of `ts_rank_cd`,
> so an entry found only by a beginning scores zero and sorts behind every entry that matched a
> whole word. `ORDER BY rank DESC, wi.id DESC` still pages them deterministically.
>
> The cost is that a one-letter search now answers instead of answering nothing, so the screen
> walks its pages for it. That is not a new shape — a filter with no words has always returned the
> whole workspace and always walked — and it is bounded the same way.

**And the tree keeps its Search row, which reverses one sentence of ADR-0063 decision 4**: "one
visible entry to search on every width". That reading treats a *field* and a *destination* as the
same thing. They are not — the field is where a search is typed, and Search is the place it is
built, which somebody goes to with nothing typed at all: to press a narrowing, to open one they were
sent, to go back to the one they were building. A reader who wants the place and is offered only a
field has to invent a search to reach it. Below `medium` nothing changes; the bottom bar has carried
Search as a destination all along, and decision 4's own compact half is untouched.

The bar's field also gets a name of its own. It and the screen's field both carried
`app.search.label`, so a screen reader asked to find "the search" found two controls with one name
and could say nothing about which.

### 5. The field sits in the middle of the bar

Where `AppBar`'s own note has claimed it sat since it was written. Two things kept it out, each
worth six pixels: the navigation toggle sat *beside* the group meant to balance the other side
rather than inside it, and a page-action slot filled with nothing still took a gap. Both are fixed
in the component, and `e2e/shell.test.mjs` measures the result to two pixels rather than looking at
it.

## Consequences

* `searchlanguages.ts`, `searchfilters.ts` and `FilterChips.svelte` are deleted. Their tests go with
  them; `searchquery.ts` carries twenty-one of its own and `menurows.ts` six.
* `ItemSearchQuery.language` stays in the contract and is simply not sent by this client. It costs
  nothing, an installation may still have a caller that means it, and removing a field from
  `openapi.yaml` is not a change a client's convenience justifies.
* Six quick narrowings are **hard-coded** in `searchquery.ts`. The honest shape is saved views, and
  `SavedView` already has `TENANT` and `ACCOUNT` scopes — Jira's starred filters exactly — but a
  saved view carries an anchor because `POST /views/{id}:export` executes it, and a workspace-wide
  search has none. That gap is a question about the contract and is deliberately left open.
* `sort:` has a place in the line and no chip: a sort is refused beside words by name, and a control
  that is dead half the time is worse than none.
* `e2e/search.test.mjs` and `e2e/shell.test.mjs` are rewritten rather than left red — the first read
  one parameter per chip, the second counted the bar's field and the tree's row as one thing. Each
  carries a case for a defect this work found, and each case was checked by putting the defect back.
* The prototype this came from — a demo server answering out of memory, a Playwright walk over it
  and a standalone SQL proof — stays out of the repository. It was built to be judged and it has
  been; what it proved is now proved by the integration suite and the engine walks, which are
  maintained. A top-level directory for scaffolding would also be a change to the project's
  structure, and that is not a decision a prototype gets to make.
* Nothing in `core/` learns that a frontend exists, and `make tokens` produces no Go diff.
