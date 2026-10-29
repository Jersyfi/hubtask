---
id: UC-CNV-03
title: Take a whiteboard out again
context: whiteboard
actors: [PE-member, PE-person]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-18]
state: built
tasks: []
checked_by: [core/application/service/whiteboard/Boards_test.go, packages/whiteboard/test/tools.test.ts, packages/whiteboard/test/geometry.test.ts, apps/webapp/src/lib/data/whiteboards.test.ts]
---

# Take a whiteboard out again

## Goal

A board leaves Hubtask whole - as a picture to show and as a file Hubtask reads back - and the workspace's export carries every board.

## Story

Carla needs the flowchart of the onboarding board in a slide, so she saves its "Flow" frame as a PNG. Before the team moves the board to another hub she saves it as a Hubtask file and makes a new board from the file there.

## How to check

1. A reader of the hub exports the board, or one of its frames, as the native file (`hubtask.whiteboard`, version 1); making a board from that file gives a board of the same objects.
2. The *Whiteboard* screen saves the whole board, the chosen frame or the chosen objects as an SVG picture and as a PNG picture, its words escaped and each card drawn with what the reader sees of its entry.
3. Every export through the API is recorded in the audit trail.
4. The workspace's archive holds every board with its objects and its history, and a restore brings them back (tenant-export.md §6).
5. The *Whiteboard* screen saves the whole board, the chosen frame or the chosen objects as a PDF of one page holding its picture.
6. A page that embeds a board as a block (a `hubtask:whiteboard/` address on a line of its own), or one frame of it (`#<frame>`), draws it live, as people draw on it, for a reader of the board's hub, and opens the board on that frame; anybody else sees why it is not shown.

## Where it ends

* The PDF holds the picture, not selectable words; no import of other tools' boards (Excalidraw, tldraw, draw.io) and no CSV of notes - the following release.
* A card exports the identifier of the entry it names, not the entry's words.
