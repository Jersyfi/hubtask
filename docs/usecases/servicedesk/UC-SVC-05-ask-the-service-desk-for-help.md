---
id: UC-SVC-05
title: Ask the service desk for help and follow my request
context: servicedesk
actors: [PE-requester, PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Portal_test.go, core/application/service/servicedesk/Portal_test.go, core/application/service/work/AddCommentPortal_test.go]
---

# Ask the service desk for help and follow my request

## Goal

A person who is not on the team asks a service for help through its catalogue, follows what becomes of the request and answers the team's questions - without seeing anybody else's requests or the team's own notes.

## Story

Mehmet holds the requester role on the collection *IT help*. The portal shows him the kinds of request it offers - *Incident* and *Access* - each with its form. He raises an incident, "The printer on floor 2 prints blank pages", answering the form's room field. He sees it in his list, reads Jonas's reply and the clock of the first response, and answers it. Jonas's internal note to Lena never reaches him.

## How to check

1. Somebody holding REQUEST at a collection reads its request catalogue: each top-level type the collection offers, not archived, whose form names the `PORTAL` channel and leaves it open, with its form without the fields the portal hides. A type whose form does not name the portal is not offered.
2. They raise a request of a type the catalogue offers: an entry in their name, held to the type's form on the `PORTAL` door; a type the catalogue does not offer is refused as `requests.type_not_offered`, and somebody without REQUEST there is refused.
3. They list the requests they raised in the collection, newest first, each with its title, notes, type, status and completion - never the team's fields, assignee or labels.
4. They read one of their requests with its public replies and how its SLA goals stand; an internal note is never in it, and a request somebody else raised is not found.
5. They answer on their own request: always a public comment, never under an internal note, never on somebody else's request.

## Where it ends

* The web app's portal screens, visibility of the requests of a whole organisation, approvals from the portal and knowledge deflection are the next parts of the service desk.

