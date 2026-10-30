---
id: UC-IMP-04
title: Move over from an issue tracker with its configuration, history and attachments
context: importer
actors: [PE-admin, PE-planner, PE-scripter]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-12, P-18]
state: built
tasks: []
checked_by: [infrastructure/importer/IssueTracker_test.go, test/integration/import_tracker_test.go, core/application/service/importer/Import_test.go, apps/webapp/src/lib/data/imports.test.ts]
---

# Move over from an issue tracker with its configuration, history and attachments

## Goal

A team leaving an issue tracker takes its projects with it as they were configured: the issue
types, the statuses with what they mean, the custom fields, every issue's history, comments, links
and attachments — so that the move is not a restart (D-10, the Data Act's switching).

## Story

An administrator fetches the projects' issues through the tracker's search with names, schema and
the change log expanded, puts the answers and the attachments into one ZIP, and chooses *Import…* on
a hub, *Jira* and the file. Each project becomes a collection that runs a workflow of the statuses
its issues were in; the issue types are entry types, a sub-task is the work under its issue, a
story under an epic implements it. Priority, components, versions and the custom fields are the
collection's fields with their values. The history says who changed what, and when, before the
move.

## How to check

1. Each project becomes a collection under the hub; each issue type an entry type at the level its
   issues sit (a sub-task's under its issue), each status a status of a workflow bound to the
   collection with its category, and every entry is in its type and status; an issue in a done
   status is complete. A sub-task whose issue the file does not carry is refused by its row.
2. Priority, components, fix versions, the issue's key, assignee and reporter (by name), and each
   custom field of a kind the product has — text, number, date, pick one, pick several, address —
   become fields of the collection with every issue's value; a custom field of no such kind is
   counted in the report.
3. The change log becomes each entry's history, a step per recorded change with both sides and the
   source's author named; comments become comments by the importing person with the author's name.
4. Links between issues become links of their type — *blocks* and *duplicates* as the shipped ones,
   any other as a workspace link type — and an issue under an epic implements it; a link to an
   issue the file does not carry is counted.
5. A ZIP carries the attachments under `attachments/<attachment id>/`: each becomes the entry's
   attachment with its bytes; one the ZIP lacks is counted.
6. The same projects imported twice create nothing the second time; a file that is not the
   tracker's answer is refused with `imports.file_not_kind`, a ZIP that cannot be opened with
   `imports.archive_unreadable`.
7. Into a collection, the entries land there and its own workflow stays: the statuses are not
   carried, the completion is.

## Where it ends

* No live connection to the tracker and no sign-in to it: the person brings the file
  ([NG-phone-home](../../vision/non-goals.md)).
* People are not matched to accounts: their names travel in text fields, comments and history.
* The workflow's screens and permission schemes are not in the search's answer, and the
  collection's roles are Hubtask's. Without the workflow answer every status is reachable from
  every other; with it, and with sprints and boards, UC-IMP-07 goes on.
* The tracker's add-on fields have no place and are counted.
* An import writes through the archive, not through each type's form: a type's required field
  does not refuse an imported entry (the suite plan's S-06 gap).
