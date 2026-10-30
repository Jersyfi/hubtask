---
id: UC-SVC-04
title: Keep notes for the team beside the replies
context: servicedesk
actors: [PE-handler, PE-requester]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11]
state: built
tasks: []
checked_by: [core/domain/model/work/Comment_test.go, core/application/service/servicedesk/Clocks_test.go]
---

# Keep notes for the team beside the replies

## Goal

A handler writes in one discussion both what the requester reads and what only the team reads, and never sends a note for the team to the person who asked by mistake.

## Story

Jonas answers Mehmet's incident: "We are on it." Below it he writes an internal note for Lena: "Check the switch in room 2 first." Lena replies to the note. Mehmet, following his request in the portal, reads Jonas's answer and nothing of the note or its reply.

## How to check

1. A comment is written public unless its author marks it internal (`internal: true`); a reply to an internal note is internal whatever it asked. Every reader of the entry in the app reads both, each marked.
2. The requester following the request in the portal reads the public comments and never an internal note (UC-SVC-05 check 4).
3. An internal note does not meet an SLA's first response; a public reply by somebody other than the requester does.

## Where it ends

* Whether a comment is internal is fixed when it is written; turning a note public later is the next release's.
