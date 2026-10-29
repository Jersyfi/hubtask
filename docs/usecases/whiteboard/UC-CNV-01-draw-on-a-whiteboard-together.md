---
id: UC-CNV-01
title: Draw on a whiteboard together
context: whiteboard
actors: [PE-member, PE-person]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-18, P-19]
state: built
tasks: []
checked_by: [core/domain/model/whiteboard/Scene_test.go, packages/whiteboard/test/tools.test.ts, core/application/service/whiteboard/Boards_test.go, test/integration/whiteboard_repository_test.go, packages/whiteboard/test/scene.test.ts, packages/whiteboard/test/geometry.test.ts]
---

# Draw on a whiteboard together

## Goal

A team thinks on a whiteboard of its hub - sticky notes, shapes, text, connectors and frames - several people at once, and finds the board again as they left it.

## Story

The team's retrospective runs on a board in the team's hub. Anna draws three frames, "Keep", "Stop" and "Try"; while she names them, Bert and Carla put amber and blue sticky notes in them, and Bert connects two notes with an arrow. A week later the board is where they left it.

## How to check

1. Whoever may write a hub makes a whiteboard there from the hub's *Whiteboards* screen and puts on it sticky notes in the ten label colours, shapes (rectangle, rounded, ellipse, decision, data, triangle), text, frames with a name, and connectors attached to two objects; reading the board back answers every object as drawn, in drawing order.
2. Moving an object keeps every connector attached to it attached at its new place.
3. A reader of the hub who may not write there sees the board and is offered no drawing; a person who may not read the hub does not find the board, and its live document is refused alike.
4. Two people drawing on the same board at the same time both keep their objects and their changes, and the board's saved objects follow what was drawn together.
5. A board holds at most 5,000 objects; a board of more is refused with `whiteboards.too_many_elements`.
6. A board put in the hub's trash is not listed or opened, and comes back from the trash with every object.
7. On the *Whiteboard* screen, Tab walks the objects and the chosen one is announced with its words, the arrows move it, N puts a sticky note in the middle and Delete takes the chosen object off.
8. Whoever may write the board draws freehand strokes with the pen or the highlighter, puts web links (http or https) drawn as their card, and groups objects; reading the board back answers each stroke's line, each link's address and each group as drawn, and a link to anything but a web address is refused with `whiteboards.link_url_invalid`.

## Where it ends

* No images or files on a board, and no comments pinned to objects - the following part of M4 (a board's files need storage of their own).
* A web link is drawn as its card; the board never loads the page (no frame of another origin).
* A board is not in the offline copy: an open board keeps being drawn on offline and merges when the connection is back; one never opened cannot be read offline.
