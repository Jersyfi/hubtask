---
id: UC-CNV-08
title: Put pictures and files on a whiteboard
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-19]
state: built
tasks: []
checked_by: [core/domain/model/whiteboard/Scene_test.go, core/application/service/whiteboard/Files_test.go, core/application/service/media/ReadMedia_test.go, test/integration/whiteboard_repository_test.go, packages/whiteboard/test/geometry.test.ts, packages/whiteboard/test/scene.test.ts]
---

# Put pictures and files on a whiteboard

## Goal

A board holds the screenshots, photos and documents a workshop needs, so that everyone drawing on it sees the pictures and opens the files without hunting for them elsewhere.

## Story

Emil drops the photo of last week's whiteboard and the PDF of the brief onto the retro board. The photo is drawn in place for everyone; the brief is a card that opens the file. Fatima copies the photo onto her own board.

## How to check

1. Whoever may draw on a board uploads a picture or a file onto it: a picture is drawn as itself, anything else as a card with the file's name, and both move, group and copy like any other object.
2. Only an upload of their own, or one that a board they may read already holds, can be placed on a board; anything else is answered as not existing, and somebody who may only read the board places nothing.
3. Whoever may read the board reads its pictures and opens its files; somebody who may not read the board's hub is told they do not exist.
4. Another workspace neither places a file on the board nor finds which boards hold one.

## Where it ends

* At most 500 files per board; the bytes count towards the workspace's storage ceiling at the upload.
* A file stays with the board as long as the board exists, even after its object is taken off - a version brought back draws it again.
* An exported SVG, PNG or PDF carries a picture's own bytes where the browser may read them back - the installation's own storage, or one of another origin that allows it; otherwise it draws the picture's place and name.
