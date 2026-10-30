---
id: UC-SVC-06
title: Answer the usual requests the usual way
context: servicedesk
actors: [PE-handler, PE-admin]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Reply_test.go, core/application/service/servicedesk/Replies_test.go, core/application/service/automation/TriggerOnEntry_test.go, test/integration/service_desk_reply_test.go, apps/webapp/src/lib/servicedesk/desk.test.ts]
---

# Answer the usual requests the usual way

## Goal

A handler answers a request the team answers every day with the reply the team agreed on, filled for this request, and does the usual steps around it with one press instead of five.

## Story

Lena writes the canned reply *Password reset* once: "Hello {{requester.name}}, your password for {{entry.title}} is reset." Jonas opens Mehmet's request, picks the reply, reads "Hello Mehmet Kaya, your password for VPN login is reset.", adds a line and sends it. Then he presses the macro *Resolve as answered*, which sets the request to *Resolved* and labels it *password*.

## How to check

1. Somebody holding STRUCTURE at the workspace defines a canned reply under a key with a name and a text; a text naming a placeholder other than `{{entry.title}}`, `{{entry.id}}`, `{{requester.name}}` and `{{agent.name}}` is refused with `reply_templates.variable_unknown`. Every member of the workspace lists them.
2. Filling a canned reply for an entry answers its text with the entry's title and identifier, the name of who raised it and of who asks; a value nobody has is left empty. It needs READ on the entry - anybody else is told the entry does not exist - and sends nothing.
3. A canned reply stays in its workspace: the workspace next door neither lists, fills nor deletes it, and it is in the workspace's archive.
4. A macro is a rule with a `MANUAL` trigger pressed on an entry (`item_id`): the run is about that entry. The presser must be allowed to change the entry, and an entry outside the rule's scope is refused with `automation.subject_outside_scope`.

## Where it ends

* A canned reply is the workspace's; one per hub or per person is not built.
