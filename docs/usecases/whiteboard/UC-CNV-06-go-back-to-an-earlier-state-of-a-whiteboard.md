---
id: UC-CNV-06
title: Go back to an earlier state of a whiteboard
context: whiteboard
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-05, P-18]
state: built
tasks: []
checked_by: [core/domain/model/whiteboard/Version_test.go, core/domain/model/lifecycle/Rule_test.go, core/application/service/lifecycle/DocumentRetention_test.go, test/integration/document_retention_test.go, core/application/service/whiteboard/Boards_test.go, test/integration/whiteboard_repository_test.go, packages/whiteboard/test/tools.test.ts]
---

# Go back to an earlier state of a whiteboard

## Goal

A board's earlier states are kept, so that a person finds how it looked before a workshop and brings that state back without losing what came after.

## Story

After the workshop the board is crowded. Carla names the version from the morning "Before the workshop", looks at it, and brings it back; the crowded state stays in the history, a click away.

## How to check

1. Every drawing session of a board leaves one version - the board's title and objects as they stood when the session rested, with who saved it last; saves closer together than ten minutes are one session, whoever saved them, and a named version is never folded into.
2. A reader of the hub lists a board's versions newest first with the moment, the number of objects and the name, and reads one with its objects; whoever may write the board names a version or takes its name away, and a reader who may not write is refused.
3. Bringing a version back puts its objects on the board in place of what stands, for everyone drawing at once; what stood before stays in the history as the version before.
4. Another workspace neither reads, names nor adds a board's versions.
5. A board's history is kept whole unless the workspace writes a `WHITEBOARD_HISTORY` rule; under
   one, an unnamed version past its period goes unless it is the board's newest, and a thread
   resolved before it goes with its replies, while a named version, an open thread and whatever a
   hold or a restricted author covers stay.

## Where it ends

* Two versions are not drawn side by side or as a difference.
* A board's history ages out only under the workspace's own rule, never a hub's, and by no default
  period.
