---
id: UC-CNV-05
title: Run a session on a whiteboard
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-05, P-18]
state: built
tasks: []
checked_by: [packages/whiteboard/test/tools.test.ts, apps/webapp/src/lib/data/whiteboards.test.ts]
---

# Run a session on a whiteboard

## Goal

A facilitator runs a workshop on a board: the participants see each other, can be brought to the same place, vote on the notes - openly or anonymously - and work against a timer everyone sees.

## Story

Bert opens the retro board; the cursors of Anna and Carla move across it with their names. He follows Anna while she walks through the frames, then starts an anonymous vote with three votes each and a five-minute timer. When the time is up, the three notes with the most votes are on top of the results, and nobody can tell who voted for what.

## How to check

1. Whoever may write the board starts a vote with a number of votes for each person, open or anonymous; each person gives at most that many votes, several on one object if they like, and takes one back; the board shows each object's votes and how many people voted, most votes first; an anonymous vote keeps no person's account with their votes; a vote ended takes no more votes and keeps its results until the next starts or they are cleared.
2. Whoever may write the board starts a timer every participant sees count down; it pauses, goes on and stops for everyone, and says when the time is up.
3. Everyone on the board - a reader as well - sees the others' cursors with their names and the list of who is there, and may follow one person's view until they move the board themselves.

## Where it ends

* No private mode (notes hidden until revealed), presentation mode, visitors by link, reactions or polls - the research's *Should* and *Later* rows.
* Votes and the timer live with the board's live drawing, not in its saved objects or its history.
