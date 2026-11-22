---
id: UC-CNV-09
title: Discuss a whiteboard where it is drawn
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-05, P-19]
state: verified
tasks: []
checked_by: [core/domain/model/whiteboard/Comment_test.go, core/application/service/whiteboard/Comments_test.go, core/application/service/notification/RecordWhiteboardMention_test.go, test/integration/whiteboard_repository_test.go, apps/webapp/src/lib/data/whiteboards.test.ts, apps/webapp/e2e/whiteboardwork.test.mjs, core/application/service/sync/InitialSyncWhiteboards_test.go, apps/webapp/src/lib/data/live.test.ts, apps/webapp/e2e/livechanges.test.mjs]
---

# Discuss a whiteboard where it is drawn

## Goal

A question about a board stands next to what it is about, and the person who can answer it hears of it, so that a workshop's open points are settled on the board rather than in a chat nobody finds again.

## Story

After the retro, Emil pins a comment to the sticky note "Deploys take too long" and names Fatima in it. Fatima gets a mail, opens the board, sees the pin at the note, answers in the thread and resolves it. When the note is taken off the board later, the thread stays where the note stood.

## How to check

1. Whoever may draw on a board pins a comment thread to one of its objects or to a place on it, and replies in a thread one level deep; whoever may read the board sees each open thread as a pin at its object - or at its place once the object is gone - and reads the threads with their replies.
2. A person the comment names who may read the board is told by a notification naming the board and the writer, which their preferences for comments decide; the writer is not told of their own words, a rewrite tells only the people it adds, and somebody who may not read the board is told nothing.
3. Whoever may draw on the board resolves or reopens a thread; only a comment's author rewrites it or takes it back, and a comment taken back keeps its place in the thread without its words. Somebody who may only read the board writes no comment.
4. Another workspace neither finds nor changes a board's comments, and the comments travel with the board in the workspace's archive and a person's copy of their data.
5. The whiteboards' home lists the open threads in which someone else names the reader, each with the object it is pinned to, and opens its board at the thread.
6. A comment written, resolved or taken back appears on every other open client that may read the board without a reload, and is in a device's copy after its next synchronisation.

## Where it ends

* A comment is not part of the live drawing: it reaches another client once it is written, never while it is typed, and who is writing one is not shown.
* At most 1,000 comments of a board are listed, and a comment names at most 20 people.
* A mention names a person of the workspace; a group or a role is not named.
