---
id: UC-PLN-09
title: Bring a plan in from a project file
context: planning
actors: [PE-planner, PE-member, PE-integrator, PE-agent]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-04, P-05, P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/planning/ProjectFile_test.go, core/application/service/planning/Import_test.go, apps/webapp/src/lib/data/planning.test.ts]
---

# Bring a plan in from a project file

## Goal

A planner who kept a plan in a desktop project tool or a spreadsheet brings it into a collection
once - its tasks, their outline, dates, milestones and dependencies - and takes the plan out again
as a table, a picture or a printed page, without retyping either way.

## Story

The construction team's schedule lives in a project tool. Mia saves it as XML and brings it into
the "Site works" collection: the dry run says 42 tasks, 12 work packages, 60 dependencies and 3
milestones; she brings them in, and the *Plan* screen shows the same chain the tool did. A week
later she exports the plan as CSV for the subcontractor and prints the chart as PDF for the site
meeting.

## How to check

1. A project tool's XML brings its tasks with their outline level, start and finish, milestone
   mark and predecessor links of every kind (FS, SS, FF, SF) with their lag in working days; the
   project's own summary task stays out.
2. A CSV with a header row and a Name column brings its tasks with, where the file has them, an
   ID, a start, a finish, an outline level, a milestone mark and a predecessors column in the
   usual notation (`3`, `3SS`, `4FF+2d`, separated by `;` or `,`).
3. A file that cannot come in whole - no name column, a predecessor the file does not hold, a date
   that is no date, not XML, more than 500 tasks or 2000 dependencies, larger than 512 KiB - is
   refused with the message saying where, and nothing is written.
4. Whoever may change a collection's entries brings a file into it: each task an entry at the depth
   its outline gives - a task, a work package under it, an activity below; a row deeper than an
   activity hangs on that activity's parent and is counted as flattened -, its dates the start and
   an all-day due date, its milestone marked, each predecessor a "blocks" dependency with its kind
   and lag. Every entry and dependency is written through its own use case, so it is checked,
   recorded and announced as one made by hand, and the file comes in whole or not at all; a hub
   takes no file (`planning.import_needs_collection`).
5. A dry run answers how many tasks, work packages, activities, dependencies and milestones the
   file brings, and how many rows it flattens, and writes nothing.
6. The *Plan* screen exports the plan as CSV - the outline level, the milestone mark and each
   entry's predecessors by row number, so the import reads it back -, the chart as PNG, and the
   chart printed, which the browser saves as PDF; a collection's *Plan* screen brings a file in
   with its dry run first.
7. REST, MCP and hubctl bring a file in alike.

## Where it ends

* A project tool's binary file is not read; the tool saves XML.
* Resources, costs, calendars and baselines of the file are not brought in; dates, outline,
  milestones and dependencies are.
* An import adds entries; it neither matches nor updates entries already in the collection.
* The PDF is the browser's print of the chart; no PDF renderer is shipped (2026-10-30).
