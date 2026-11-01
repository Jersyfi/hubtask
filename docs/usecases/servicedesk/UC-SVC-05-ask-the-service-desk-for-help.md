---
id: UC-SVC-05
title: Ask the service desk for help and follow my request
context: servicedesk
actors: [PE-requester, PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: verified
tasks: []
checked_by: [core/domain/model/servicedesk/Portal_test.go, core/domain/model/work/EntryTypeText_test.go, core/application/service/servicedesk/PortalCatalogue_test.go, core/application/service/workflow/EntryTypes_test.go, test/integration/entry_type_texts_test.go, apps/webapp/src/lib/portal/portal.test.ts, apps/webapp/src/lib/data/entrytypes.test.ts, core/application/service/notification/RecordNotifications_test.go, core/application/service/notification/DeliverNotification_test.go, core/application/service/access/EmergencyAccess_test.go, core/application/service/servicedesk/Portal_test.go, core/application/service/work/AddCommentPortal_test.go, core/domain/model/media/Object_test.go, core/application/service/media/ReadMedia_test.go, core/application/service/servicedesk/DeskLogo_test.go, test/integration/desk_logo_test.go]
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
6. A public reply of the team on their request tells them under their own notification category, `REQUEST_REPLY`, which they may switch off: in the comment's words, with a link to the request in the portal, never to the entry's screen. An internal note tells them nothing, nor do their own words; whether they are a requester is read when the reply is written.

7. The catalogue names and describes each type in the reader's language where the type's texts say it there - the exact language tag first, then its language alone (`de` for `de-AT`), then the type's default name and description, field by field; an administrator writes these texts per language on the type, at most 20 languages, and a tag that is no language is refused as `entry_types.language_malformed`.
8. The portal's head shows the service it is for: the collection's name, description, icon and colour, as the catalogue answers them; the portal's own words stand where the collection has no description.
9. Somebody holding STRUCTURE at the collection gives the desk a logo of their own upload - a PNG, JPEG, GIF or WebP image, at most 5 MB; anything else is refused as `media.logo_not_an_image` - and the portal's head draws it in place of the icon for every requester, who may read it as they may raise a request there; removed, the icon and colour stand again.
10. A request the team merged into another (UC-SVC-03 check 5) names the surviving one when its
    requester reads it, through the `duplicates` link: where it stands, the team's public replies
    and how its SLA goals stand - never an internal note, the team's fields or what the surviving
    request's own requester wrote. Where the surviving request lies at a desk the reader may not
    raise requests at, only its number is named. Its public replies tell them as their own do
    (UC-SVC-03 check 8). By the owner's decision (accepted.md, "The owner's fourth answers",
    item 7).

## Where it ends

* The requests of a whole organisation are UC-SVC-11; approvals from the portal are UC-SVC-09.
* A desk's articles, in each language they are written in, are UC-DOC-13.

