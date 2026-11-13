---
id: UC-WRK-27
title: Find and do anything from one place
context: work
actors: [PE-person, PE-member, PE-admin, PE-owner]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-13]
state: built
tasks: [UX-13]
checked_by: [apps/webapp/e2e/palette.test.mjs, apps/webapp/src/lib/search/everything.test.ts, apps/webapp/e2e/engines.test.mjs, apps/webapp/e2e/search.test.mjs, apps/webapp/src/lib/palette/sources.test.ts, apps/webapp/src/lib/palette/rows.test.ts, apps/webapp/src/lib/palette/registry.test.ts, apps/webapp/src/lib/palette/actions.test.ts, apps/webapp/src/lib/shortcuts.test.ts]
---

# Find and do anything from one place

## Goal

A person who knows what they want gets there by typing it: an entry, a collection, a setting or an action, without knowing in which menu it lives.

## Story

The person presses `⌘K`, types "kitch" and sees the collection "Kitchen", entries in it and the actions "New task in Kitchen" and "Open Kitchen as a board".

## How to check

1. `⌘K` (`Ctrl K`) and the search field in the bar open one palette on every page.
2. It lists entries, hubs and collections the person can see, settings pages they may open, and actions they may perform here, ranked by match; nothing they may not do is listed (P-05).
3. Every action listed shows its keyboard shortcut where it has one; `?` opens the list of all shortcuts.
4. The quick filters of the bar search (mine and open, due this week, overdue, nobody on it, changed this week, recently finished) stay available in the palette.
5. Enter performs the highlighted item; Escape closes the palette and returns focus where it was.
6. Typed words search every kind the person may read (UC-WRK-18 check 9): over the hits stand scope chips with
   counts - all, each kind that found something, tickets apart where the service desk is on, *here: hub* where one
   stands in a hub - and Tab moves through them; the best hits come first, one of each other kind after them
   (work surface §6.11).
7. A typed key such as `SA-12` names its entry as the first row, Enter opens it; `⌘↵` shows every hit on the
   search page; a line that starts with `>` lists commands only.
8. Before anything is typed the palette shows what was opened recently, the person's saved searches and the quick
   commands.

## Where it ends

* Asking the workspace a question is [UC-AI-08](../suggestion/UC-AI-08-ask-the-workspace-a-question.md).
* The full search page with its filters stays [UC-WRK-18](./UC-WRK-18-find-an-entry-again.md).

