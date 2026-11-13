---
id: UC-WRK-18
title: Find an entry again by searching
context: work
actors: [PE-person, PE-member, PE-guest, PE-child, PE-agent, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-08, P-12, P-13, P-14]
state: built
tasks: [C-08, J-09, J-10, M-09, F3-18, F10-05, F10-18]
checked_by: [core/application/service/search/Doors_test.go, core/application/service/servicedesk/Queues_test.go, core/domain/model/view/SavedView_test.go, test/integration/search_test.go, test/integration/search_names_test.go, core/application/service/search/Search_test.go, core/application/service/whiteboard/Found_test.go, core/application/service/goal/Found_test.go, core/application/service/cmdb/Found_test.go, core/application/service/identity/FoundPeople_test.go, core/application/service/work/FoundOnEntries_test.go, test/integration/page_repository_test.go, test/integration/hybrid_search_test.go, test/integration/search_model_test.go, core/application/service/work/SearchItems_test.go, apps/webapp/e2e/search.test.mjs, test/security/search_test.go]
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
9. One search answers every kind beside entries - tickets (the entries of a desk), pages,
   whiteboards, goals, dashboards, inventory items, people, file names and comments - each hit with its kind, title, place, tile, the passage
   that matched where there is one and its state, and a count per kind; every hit and every count
   holds only what the person may read, people are found by their name alone and a file by its
   name alone (`POST /search:all`, work surface E-11).
10. Among equally good pages a page verified now comes first, and a page due for review says so;
    the order changes, not what is found (work surface E-8).
11. A ticket key typed whole, `SA-12`, finds that entry first - by the collection's key prefix or
    its handle - where the person may read it, and nothing where they may not (work surface §6.11).
12. A search is saved as a view that keeps its line as it was typed; a line that does not read as a
    query is refused where it is saved, and opening the view runs the line again with the reader's
    permissions (work surface §6.11, Q-5).

## Where it ends

* No search across workspaces ([P-01](../../vision/principles.md)).
* No search inside attachments' content: an attachment is found by the name it arrived under,
  never by its text, because reading a file's text needs an extractor the product does not carry.
* No saved searches separate from saved views
  ([UC-WRK-17](./UC-WRK-17-filter-sort-and-save-a-view-of-my-work.md)).
* Rebuilding the index after a language configuration changed is an administrator's action and
  belongs to the workspace's administration.
* No page numbers ([NG-page-numbers](../../vision/non-goals.md)); AI is never required
  ([NG-ai-required](../../vision/non-goals.md)).
