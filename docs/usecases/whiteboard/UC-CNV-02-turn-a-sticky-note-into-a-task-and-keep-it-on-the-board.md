---
id: UC-CNV-02
title: Turn a sticky note into a task and keep it on the board
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18, P-19]
state: built
tasks: []
checked_by: [packages/whiteboard/test/scene.test.ts, core/application/service/work/LinksWhiteboard_test.go, test/integration/whiteboard_repository_test.go, core/domain/model/whiteboard/Scene_test.go]
---

# Turn a sticky note into a task and keep it on the board

## Goal

A note on a whiteboard becomes a task in a collection of the hub, and stays on the board as that task's live card; any entry can be put on a board as its card, and the entry knows the boards it is on.

## Story

At the end of the retro, Anna turns the note "Book the room earlier" into a task in the team's "Admin" collection. The note becomes the task's card in its place, with its arrow still attached. When Bert completes the task on the board's card, the task is completed everywhere; on the task, "On whiteboard: Retro" leads back to the board.

## How to check

1. Whoever may write the board turns a sticky note with words into a `TASK` in a collection of the board's hub they choose; the note's words become the task's title, and the note becomes the task's card in its place, its connectors still attached. A note without words is not turned.
2. Whoever may write the board puts an entry on it as a card, chosen from the entries they can see.
3. A card shows its entry's key, title and completion as the reader may see the entry, and an entry the reader may not see as not available; ticking a card completes or reopens the entry, and a refusal is shown in words.
4. An entry's links name the boards drawing it as a card or linking to it (`whiteboard_backlinks`), each only where the reader may read the board's hub; a board in the trash names none.
5. A reference to a board (`hubtask:whiteboard/<id>`, `#<frame>` for one frame) resolves to its title and number of objects for a reader of its hub, and to its address alone for anybody else.

## Where it ends

* One note at a time, made a `TASK`; no turning a frame of notes into entries at once, and no other type chosen on the board.
* A connector between two cards is a line on the board, not a dependency between the entries.
