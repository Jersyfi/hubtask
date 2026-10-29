---
id: UC-CNV-07
title: See what points at a whiteboard
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-19]
state: built
tasks: []
checked_by: [core/application/service/whiteboard/Backlinks_test.go, apps/webapp/src/lib/data/whiteboards.test.ts]
---

# See what points at a whiteboard

## Goal

A board says where it is used, so that a person who finds it knows which pages, entries and other boards rely on it before changing or trashing it.

## Story

Dana wants to tidy up the "Roadmap" board. Its side panel shows that the planning page embeds it, that a task's notes mention it and that the quarterly board carries it as a card; she opens the page first.

## How to check

1. A reader of the board's hub lists what points at it - the pages, entries' notes and comments that mention it, show it as a card or embed it, and the other boards that carry it - each with its title, how it points and a way to open it.
2. What sits where the reader may not read - a page or board in a hub out of reach, an entry they may not see - is left out, and the list says nothing of it.
3. A reader who may not read the board's hub learns nothing, not even that the board is pointed at.

## Where it ends

* A trashed page, entry or board points nowhere until it is restored.
* At most 200 sources of each kind are listed.
