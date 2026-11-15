---
id: UC-WRK-43
title: Ask the same question everywhere with one query language
context: work
actors: [PE-member, PE-admin, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-19]
state: verified
tasks: []
checked_by: [apps/webapp/src/lib/search/queryline.test.ts, apps/webapp/e2e/search.test.mjs, test/querylanguage/lines_test.go, core/domain/model/view/Query_test.go, core/application/service/work/QueryItems_test.go, core/application/service/work/SearchItems_test.go, core/application/service/work/AggregateItems_test.go, packages/query/test/lines.test.ts, packages/query/test/language.test.ts, apps/webapp/e2e/viewpanel.test.mjs]
---

# Ask the same question everywhere with one query language

## Goal

A person writes a question once - `who: me, none is: open due: <today` - and the same words narrow a list, a queue, a search, a dashboard widget and a call of the API, MCP or `hubctl`, with the same answer, never more than they may read.

## Story

Mara types `type: Störung who: me, none -label: intern` into her queue's line; each term becomes a chip, the count of matching tickets stands beside the line before she presses Enter, and a typo stands at its chip with the word that fixes it. She saves the line as a view; Tom pastes the same text into `hubctl item query` and an agent sends it through MCP, and both get the tickets they may read. An integrator who prefers `status = open AND who = me ORDER BY due_at` writes that spelling; it is read the same way.

In `D1` and `D2` a person mostly uses the line on their own lists; in `D4`–`D7` shared views and agents carry the same text across teams.

## How to check

1. A query in the `field: value` spelling and the same query in the `=`/`AND`/`ORDER BY` spelling are read into the same filter tree and order, and the `field: value` text written back from a tree reads into the same query; the server and the browser read every line of `test/querylanguage/lines.json` alike.
2. `GET /meta/capabilities` lists under `query_keys` only keys the server reads, each with its kind of value, its product prefix and examples that read; names are the client's, the keys stay English.
3. `POST /items:query` reads `q` in both spellings; a key or value it cannot read is refused with `query.field_unknown` or `query.value_unknown`, the character it stops at and a suggestion.
4. `POST /search` takes the search as one `query`: its free words are ranked, the rest narrows; sent beside `q` it is refused with `query.line_with_filter`.
5. The count of a line while it is typed and a key's values with their counts are `POST /items:aggregate` with `q` over the reader's own readable scope, so a member never sees a count or a value of an entry they may not read; the line travels in the body, never in an address.
6. A line is one row of chips: a term is a chip, a group or a word a text chip, and a refused term carries the problem and its suggestion; the search page draws it as its one line, with no switch to a second editor, and a `word:` that names no key is searched as words and said so at the line.

## Where it ends

* Which kinds of object beyond entries a search reads (pages, boards, goals, dashboards, inventory, people, file names, comments) is UC-WRK-18's; this one is the language they share.
* Formulas over values stay CEL (E-12); a question in words that writes the line is the AI's (Q-6, P-14).
