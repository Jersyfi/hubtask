---
id: UC-IMP-07
title: Bring an issue tracker's transitions, sprints and boards along
context: importer
actors: [PE-admin, PE-person]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-12]
state: built
tasks: []
checked_by: [infrastructure/importer/IssueTrackerPlanning_test.go]
---

# Bring an issue tracker's transitions, sprints and boards along

## Goal

A team moving over from an issue tracker (UC-IMP-04) keeps how it worked, not only what it worked
on: the workflow's transitions, its boards and its sprints arrive with the projects (D-10).

## Story

The administrator puts the tracker's workflow answer and the boards' configurations into the ZIP
beside the search answers and imports it as in UC-IMP-04. The collection's workflow allows the
steps the tracker allowed, its board view shows the columns in the board's order, and *Sprints*
lists the closed, the running and the planned sprints with the entries in each.

## How to check

1. With the workflow answer, the tracker's transitions become the workflow's steps: a directed one
   from the statuses it names, a global one from any status, the initial one the initial status;
   a transition to a status the import does not carry is counted, and a set of transitions that
   would leave a status unreachable keeps every status reachable from any other.
2. A board's columns order the workflow's statuses, and each board on the project becomes a board
   view of the collection by status, shared with the collection's members.
3. The issues' sprints become the collection's sprints with their name, goal, days and state, and
   each issue's place in them: in a closed sprint done, carried into a later sprint or returned;
   in the running or a planned sprint still open, in one sprint at a time.
4. Story points become the entries' estimates in points, a closed sprint's committed and completed
   totals count them; into a collection that runs its own sprints, no imported sprint is running.

## Where it ends

* A board's filter, swimlanes and quick filters are not carried: the board view shows the whole
  collection.
* A board column that holds several statuses becomes one column per status.
* A transition's conditions, validators and screens are not carried; the steps are open to
  everyone who may edit the entry.
