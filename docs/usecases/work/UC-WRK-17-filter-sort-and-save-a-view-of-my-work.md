---
id: UC-WRK-17
title: Filter, sort and save a view of my work
context: work
actors: [PE-person, PE-member, PE-admin, PE-scripter, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-09, P-11, P-12, P-13]
state: partial
tasks: [B-12, D-07, D-08, F2-13, F3-15, F10-14]
checked_by: [test/integration/query_language_test.go, test/integration/query_test.go, test/integration/saved_view_test.go, test/integration/saved_view_acceptance_test.go, test/integration/export_test.go, core/domain/model/view/Filter_test.go, core/domain/model/view/SavedView_test.go, apps/webapp/e2e/container.test.mjs, apps/webapp/e2e/timeline.test.mjs]
---

# Filter, sort and save a view of my work

## Goal

A person narrows a collection to what matters now — mine, open, due this week, labelled "urgent"
— orders it, looks at it as a list, a board or a timeline, and keeps that as a named view for
themselves or for everybody in the collection; and can take the result away as a file.

## Story

In the "Orders" collection the person opens the filter, adds *assignee is me*, *completed is no*
and *priority is high*, sorts by due date and switches to the board. They save it as "My urgent
orders", visible only to them. The team lead saves "This week" for the whole collection on the
timeline. Anybody applying "This week" sees its filter in the filter panel and can adjust it
before saving a new one. At the end of the month the lead exports the view as CSV.

## How to check

1. The filter builds conditions from the fields the server publishes — including the collection's
   custom fields — with the operators each field allows; conditions combine, and the count of
   active conditions shows on the button that opens it.
2. The list is sorted by one chosen field and direction, or by the collection's own order when none
   is chosen; the board is grouped by column or by another field that allows grouping.
3. The layout switch offers the list (collapsed or expanded), the board and the timeline.
4. A view is saved with a name, its conditions, its order, its layout and its grouping — for the
   person alone, or for the collection, the hub or the workspace; anything wider than the person
   needs the right to change that scope's shape.
5. A view shared with its scope is listed for everyone who can see that scope; a private one only
   for its owner. A public link is refused.
6. Applying a saved view sets the list, the layout **and the filter panel** to the view's
   conditions, so that the person sees what the view asks and can change it.
7. A view saved for the hub from a collection's page is saved on that hub.
8. A view is exported as CSV, JSON or a calendar file (ICS) holding only what the person may read;
   an export cut off at the row limit (5 000) says so.
9. A condition that names an unknown field, an operator the field does not allow, or a value of
   the wrong kind is refused with a code naming the condition; no text of a request ever becomes
   query text ([ADR-0026](../../adr/ADR-0026-query-dsl-sql-construction.md)).

## Where it ends

* No page numbers — lists scroll and load on ([NG-page-numbers](../../vision/non-goals.md)).
* No public link to a view: `PUBLIC_LINK` is declared in the contract and refused by this version
  (`core/domain/model/view/SavedView.go`).
* No choosing which fields a list row shows in the web app; `visible_fields` is stored for clients
  that use it.
* No sorting by a custom field, and no sort by more than four keys through the API.
* Subscribing to a view from a calendar is the integration context's.
* The layout a person switches to without saving a view is not remembered across visits.

## Today

* Check 6: not met — applying a view sets the query and the layout, but the filter panel keeps its own empty state: the view's conditions are not shown, and touching the filter replaces them.
* Check 7: not met — saving with the hub scope sends the collection's id, which the server refuses with `views.scope_container_mismatched`, tracked in #1082.
