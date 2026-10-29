---
id: UC-CNV-04
title: Arrange a whiteboard quickly
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-18]
state: built
tasks: []
checked_by: [packages/whiteboard/test/tools.test.ts, core/application/service/whiteboard/Boards_test.go]
---

# Arrange a whiteboard quickly

## Goal

A person lays out a board quickly and tidily: drawing by hand, lining objects up, finding their way on a large board, reusing what is already drawn, and starting from a template rather than an empty board.

## Story

Anna starts the team's retro from the "Start, stop, continue" template, circles two notes with the highlighter and drags a third until its edge snaps to its neighbour. She copies the "Actions" frame with its notes and pastes it on the planning board. On the large planning board she clicks the map in the corner to jump to the far end.

## How to check

1. Whoever may write the board draws with the pen or the highlighter; the line is kept as one or more strokes, each within its own box, and the eraser takes off a stroke it touches.
2. Moving objects, an edge or centre that comes near another object's snaps to it and the guide is drawn; with the grid on, objects land on the grid instead.
3. The map of the board shows every object and the part of the board on the screen; clicking it moves the screen there.
4. Objects grouped are chosen, moved and copied together; copied objects - with the connectors between them - paste on the same or another board under new identifiers, the connectors attached to the copies, the group a new one; Ctrl or ⌘ with C, X, V and D copy, cut, paste and duplicate.
5. The templates "Retrospective: start, stop, continue", "Retrospective: liked, learned, lacked, longed for", "Retrospective: mad, sad, glad", "Kanban board", "Brainstorm" and "Flowchart" put their frames, notes, shapes and connectors on the board in the reader's language.

## Where it ends

* No shape libraries, icon sets, tables, mind-map nodes with automatic layout or diagram types beyond the flowchart's shapes - the research's *Should* rows.
* No template a workspace makes and governs itself.
