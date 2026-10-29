---
id: UC-DOC-03
title: Turn a line of a page into a task and keep them linked
context: knowledge
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-18, P-19]
state: built
tasks: []
checked_by: [packages/editor/test/tasks.test.ts]
---

# Turn a line of a page into a task and keep them linked

## Goal

A checklist line of a page becomes a task in a collection of the hub, and from then on the line and the task are one: the line reads as the task's title and its tick is the task's completion.

## Story

The meeting notes list "Book the train" as an open point. Anna turns the line into a task in the team's "Travel" collection. Bert completes the task on the board; the next time the notes are open, the line is ticked. Ticking another linked line completes its task.

## How to check

1. Whoever may write the page turns the checklist line the caret stands in into a `TASK` in a collection of the page's hub they choose; the line then mentions the entry, so it reads as the entry's title and the entry lists the page among its backlinks (UC-DOC-04).
2. A linked line is drawn ticked when its entry is completed and unticked when it is open; ticking or unticking the line completes or reopens the entry, and a refusal is shown in words.

## Where it ends

* The tick is mirrored when the page is open; the page's saved Markdown, its search and its export carry the tick as last drawn.
* No conversion of several lines at once, and no task made from a line that is not a checklist line.
