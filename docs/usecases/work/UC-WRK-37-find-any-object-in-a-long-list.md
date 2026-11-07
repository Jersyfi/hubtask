---
id: UC-WRK-37
title: Find any object in a long list and act on it
context: work
actors: [PE-admin, PE-auditor, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-10, P-12, P-13]
state: partial
tasks: []
checked_by: [apps/webapp/build/pagekinds.test.js, apps/webapp/e2e/pagesystem.test.mjs]
---

# Find any object in a long list and act on it

## Goal

On any list of objects a person narrows the list to what they look for, sees how many are shown, and acts on a row without leaving the list or meeting a form first.

## Story

Jana looks for Priya among the people. She types "pri" into the search above the table; the count says *1 of 9*. The row has one quiet verb and a ⋯ with the rest. On her phone the filters are behind *Filters*, and the nine people fit in one and a half screens.

In `D1` a list holds a handful of rows and the search is rarely needed; in `D4`–`D7` lists hold hundreds of rows and the foot continues them.

## How to check

1. Every list page and log page has a search, its filters and the count above its rows; at 390×844 the filters are behind one *Filters* button.
2. No filter form is open on arrival.
3. The foot of the list says how many rows are shown and continues the list where there are more.
4. A row shows at most one verb and a menu with the rest.
5. At 390×844 the *People* page with the seeded nine people is at most 1.5 screens high, its head included.

## Where it ends

* The total behind *Showing n of y* needs a count from the API (page-system decision D-1); until it lands the foot says *Showing n*.
* Narrowing that the API cannot answer yet narrows what is loaded.

## Today

* Check 1: not met — the other list and log pages have no filter bar yet; met on *People & access*, tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 2: not met — the audit, the workspaces and the rule runs still open a filter form on arrival; met on *People & access*, tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 3: not met — the other list pages have no foot saying what is shown yet; met on *People & access*, tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
* Check 4: not met — the other list pages still show several verbs per row; met on *People & access*, tracked in the page-system concept, `docs/backlog/page-system/README.md` §13 steps 5-10
