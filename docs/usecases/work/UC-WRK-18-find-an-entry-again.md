---
id: UC-WRK-18
title: Find an entry again by searching
context: work
actors: [PE-person, PE-member, PE-guest, PE-child, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-08, P-12, P-13, P-14]
state: built
tasks: [C-08, J-09, J-10, M-09, F3-18, F10-05, F10-18]
checked_by: [test/integration/search_test.go, test/integration/hybrid_search_test.go, test/integration/search_model_test.go, core/application/service/work/SearchItems_test.go, apps/webapp/e2e/search.test.mjs]
---

# Find an entry again by searching

## Goal

A person who remembers a word or two — or only that it was "something about the boiler, due last
month" — finds the entry from anywhere in the workspace, sees only what they are allowed to see,
and the search works the same with AI switched off.

## Story

In the bar the person types "boil"; after a short pause the five best matches drop down, "Boiler
service" among them. Enter leads to the search page, where they narrow with chips — *mine*,
*open*, *overdue* — or by typing `due:month label:house`. With a provider consented to, a switch
lets the search also match by meaning ("heating" finds "boiler"); the words still win.

## How to check

1. Typing in the bar shows up to five matches after a short pause; an unfinished last word matches
   as the start of a word ("boil" finds "Boiler").
2. Words are matched in the entry's own language, including inflected forms, in title and notes.
3. The search page takes narrowings as chips or as `key:value` words — type, responsible person,
   member, creator, due, start, created, updated, done, collection, label, title, notes, order —
   and the two stay the same state; a narrowing with no words is a search too.
4. The words are never put in the page's address; a reload keeps them, and a shared link carries
   the narrowing and none of the words.
5. The results hold only entries the person may read; an entry on whose path they hold no role
   never appears, and nothing on the page hints that it exists.
6. Archived and trashed entries appear only when asked for (`is:archived`, `is:trashed`).
7. *By meaning* is shown only where the installation offers it — a provider the workspace
   consented to, and the vector extension; without it the search is words only and nothing on the
   page looks missing or broken.
8. Results are one continuous list without page numbers; when there are more matches than the
   page reads (500), the page says the list is not complete.

## Where it ends

* No search across workspaces ([P-01](../../vision/principles.md)).
* No search inside attachments' content.
* No saved searches separate from saved views
  ([UC-WRK-17](./UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md)).
* Rebuilding the index after a language configuration changed is an administrator's action and
  belongs to the workspace's administration.
* No page numbers ([NG-page-numbers](../../vision/non-goals.md)); AI is never required
  ([NG-ai-required](../../vision/non-goals.md)).
